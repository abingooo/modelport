//go:build unit

package handler

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	middleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type invalidRequestFallbackGroupRepo struct {
	service.GroupRepository
	groups map[int64]*service.Group
}

type invalidRequestFallbackSchedulerCache struct {
	*fakeSchedulerCache
}

func (c *invalidRequestFallbackSchedulerCache) GetSnapshot(_ context.Context, bucket service.SchedulerBucket) ([]*service.Account, bool, error) {
	accounts := make([]*service.Account, 0, len(c.accounts))
	for _, account := range c.accounts {
		if account == nil {
			continue
		}
		for _, membership := range account.AccountGroups {
			if membership.GroupID == bucket.GroupID {
				accounts = append(accounts, account)
				break
			}
		}
	}
	return accounts, true, nil
}

func (r *invalidRequestFallbackGroupRepo) GetByID(_ context.Context, id int64) (*service.Group, error) {
	return r.groups[id], nil
}

func (r *invalidRequestFallbackGroupRepo) GetByIDLite(ctx context.Context, id int64) (*service.Group, error) {
	return r.GetByID(ctx, id)
}

type invalidRequestFallbackUserRepo struct {
	service.UserRepository
	mu      sync.Mutex
	balance float64
	reads   int
}

func (r *invalidRequestFallbackUserRepo) GetByID(_ context.Context, id int64) (*service.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	return &service.User{ID: id, Balance: r.balance}, nil
}

func (r *invalidRequestFallbackUserRepo) setBalance(balance float64) {
	r.mu.Lock()
	r.balance = balance
	r.mu.Unlock()
}

func (r *invalidRequestFallbackUserRepo) readCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reads
}

type invalidRequestFallbackUpstream struct {
	mu                sync.Mutex
	antigravityID     int64
	anthropicID       int64
	calls             map[int64]int
	onAntigravityCall func()
}

func (u *invalidRequestFallbackUpstream) Do(_ *http.Request, _ string, accountID int64, _ int) (*http.Response, error) {
	return u.response(accountID), nil
}

func (u *invalidRequestFallbackUpstream) DoWithTLS(_ *http.Request, _ string, accountID int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.response(accountID), nil
}

func (u *invalidRequestFallbackUpstream) response(accountID int64) *http.Response {
	u.mu.Lock()
	u.calls[accountID]++
	callback := u.onAntigravityCall
	isAntigravity := accountID == u.antigravityID
	u.mu.Unlock()

	if isAntigravity {
		if callback != nil {
			callback()
		}
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"X-Request-Id": []string{"req-prompt-too-long"}},
			Body:       io.NopCloser(bytes.NewBufferString(`{"error":{"message":"Prompt is too long"}}`)),
		}
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(bytes.NewBufferString(`{
			"id":"msg_fallback","type":"message","role":"assistant","model":"claude-sonnet-4-5",
			"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","stop_sequence":null,
			"usage":{"input_tokens":8,"output_tokens":3}
		}`)),
	}
}

func (u *invalidRequestFallbackUpstream) callCount(accountID int64) int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.calls[accountID]
}

func newInvalidRequestFallbackHandler(
	t *testing.T,
	groups map[int64]*service.Group,
	accounts []*service.Account,
	userRepo *invalidRequestFallbackUserRepo,
	upstream *invalidRequestFallbackUpstream,
) *GatewayHandler {
	t.Helper()

	schedulerCache := &invalidRequestFallbackSchedulerCache{
		fakeSchedulerCache: &fakeSchedulerCache{accounts: accounts},
	}
	schedulerSnapshot := service.NewSchedulerSnapshotService(schedulerCache, nil, nil, nil, nil)
	groupRepo := &invalidRequestFallbackGroupRepo{groups: groups}
	gatewayCfg := &config.Config{RunMode: config.RunModeSimple}
	gatewayService := service.NewGatewayService(
		nil,
		groupRepo,
		nil,
		nil,
		userRepo,
		nil,
		nil,
		nil,
		gatewayCfg,
		schedulerSnapshot,
		nil,
		nil,
		nil,
		nil,
		nil,
		upstream,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	antigravityService := service.NewAntigravityGatewayService(
		nil,
		nil,
		nil,
		&service.AntigravityTokenProvider{},
		nil,
		upstream,
		nil,
		nil,
	)
	billingCfg := &config.Config{}
	billingCacheService := service.NewBillingCacheService(nil, userRepo, nil, nil, nil, nil, billingCfg, nil)
	t.Cleanup(billingCacheService.Stop)

	concurrencyService := service.NewConcurrencyService(&fakeConcurrencyCache{})
	return &GatewayHandler{
		gatewayService:            gatewayService,
		antigravityGatewayService: antigravityService,
		billingCacheService:       billingCacheService,
		concurrencyHelper:         NewConcurrencyHelper(concurrencyService, SSEPingFormatClaude, 0),
		maxAccountSwitches:        1,
		maxAccountSwitchesGemini:  1,
		cfg:                       gatewayCfg,
	}
}

func TestGatewayHandlerMessages_InvalidRequestFallbackRechecksTargetFreeGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const (
		originalGroupID = int64(2201)
		fallbackGroupID = int64(2202)
		antigravityID   = int64(1201)
		anthropicID     = int64(1202)
		userID          = int64(4201)
	)

	tests := []struct {
		name                 string
		originalFree         bool
		fallbackFree         bool
		initialBalance       float64
		exhaustAfterAG       bool
		wantStatus           int
		wantAnthropicCalls   int
		wantBalanceReadCount int
	}{
		{
			name:                 "free original does not exempt paid fallback",
			originalFree:         true,
			initialBalance:       0,
			wantStatus:           http.StatusForbidden,
			wantAnthropicCalls:   0,
			wantBalanceReadCount: 1,
		},
		{
			name:                 "free fallback ignores balance exhausted after paid admission",
			fallbackFree:         true,
			initialBalance:       1,
			exhaustAfterAG:       true,
			wantStatus:           http.StatusOK,
			wantAnthropicCalls:   1,
			wantBalanceReadCount: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fallbackID := fallbackGroupID
			originalGroup := &service.Group{
				ID:                              originalGroupID,
				Hydrated:                        true,
				Platform:                        service.PlatformAnthropic,
				Status:                          service.StatusActive,
				IsFree:                          tc.originalFree,
				FallbackGroupIDOnInvalidRequest: &fallbackID,
			}
			fallbackGroup := &service.Group{
				ID:       fallbackGroupID,
				Hydrated: true,
				Platform: service.PlatformAnthropic,
				Status:   service.StatusActive,
				IsFree:   tc.fallbackFree,
			}
			antigravityAccount := &service.Account{
				ID:          antigravityID,
				Name:        "antigravity-invalid-request",
				Platform:    service.PlatformAntigravity,
				Type:        service.AccountTypeOAuth,
				Status:      service.StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
				Credentials: map[string]any{
					"access_token": "ag-token",
					"project_id":   "ag-project",
				},
				Extra: map[string]any{"mixed_scheduling": true},
				AccountGroups: []service.AccountGroup{
					{AccountID: antigravityID, GroupID: originalGroupID},
				},
			}
			anthropicAccount := &service.Account{
				ID:          anthropicID,
				Name:        "anthropic-invalid-request-fallback",
				Platform:    service.PlatformAnthropic,
				Type:        service.AccountTypeAPIKey,
				Status:      service.StatusActive,
				Schedulable: true,
				Concurrency: 1,
				Priority:    1,
				Credentials: map[string]any{"api_key": "sk-test"},
				AccountGroups: []service.AccountGroup{
					{AccountID: anthropicID, GroupID: fallbackGroupID},
				},
			}

			userRepo := &invalidRequestFallbackUserRepo{balance: tc.initialBalance}
			upstream := &invalidRequestFallbackUpstream{
				antigravityID: antigravityID,
				anthropicID:   anthropicID,
				calls:         make(map[int64]int),
			}
			if tc.exhaustAfterAG {
				upstream.onAntigravityCall = func() { userRepo.setBalance(0) }
			}
			h := newInvalidRequestFallbackHandler(
				t,
				map[int64]*service.Group{
					originalGroupID: originalGroup,
					fallbackGroupID: fallbackGroup,
				},
				[]*service.Account{antigravityAccount, anthropicAccount},
				userRepo,
				upstream,
			)

			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			body := []byte(`{
				"model":"claude-sonnet-4-5","max_tokens":32,"stream":false,
				"messages":[{"role":"user","content":"hello"}]
			}`)
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req = req.WithContext(context.WithValue(req.Context(), ctxkey.Group, originalGroup))
			c.Request = req

			apiKey := &service.APIKey{
				ID:      3201,
				UserID:  userID,
				GroupID: func() *int64 { id := originalGroupID; return &id }(),
				Status:  service.StatusActive,
				User:    &service.User{ID: userID, Concurrency: 10, Balance: tc.initialBalance},
				Group:   originalGroup,
			}
			c.Set(string(middleware.ContextKeyAPIKey), apiKey)
			c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: userID, Concurrency: 10})

			h.Messages(c)

			require.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
			require.Equal(t, 1, upstream.callCount(antigravityID))
			require.Equal(t, tc.wantAnthropicCalls, upstream.callCount(anthropicID))
			require.Equal(t, tc.wantBalanceReadCount, userRepo.readCount())
		})
	}
}
