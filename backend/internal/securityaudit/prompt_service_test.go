package securityaudit

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type staticSettingRepository struct {
	values map[string]string
}

func (r staticSettingRepository) Get(context.Context, string) (*service.Setting, error) {
	return nil, service.ErrSettingNotFound
}
func (r staticSettingRepository) GetValue(context.Context, string) (string, error) {
	return "", service.ErrSettingNotFound
}
func (r staticSettingRepository) Set(context.Context, string, string) error { return nil }
func (r staticSettingRepository) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		result[key] = r.values[key]
	}
	return result, nil
}
func (r staticSettingRepository) SetMultiple(context.Context, map[string]string) error { return nil }
func (r staticSettingRepository) GetAll(context.Context) (map[string]string, error) {
	return r.values, nil
}
func (r staticSettingRepository) Delete(context.Context, string) error { return nil }

func TestPromptServiceHasExplicitIdempotentLifecycle(t *testing.T) {
	config := NewConfigManager(nil, staticSettingRepository{values: map[string]string{
		SettingKeyPromptAuditConfig: "",
		SettingKeyRiskControl:       "false",
	}}, nil, prefixEncryptor{}, testTotpKeyConfig())
	service := NewPromptService(
		config,
		NewPostgreSQLRepository(nil),
		NewRedisPayloadStore(nil),
		NewOpenAICompatibleScanner(),
		NewAtomicMetrics(),
	)

	require.Nil(t, service.cancel, "construction must not start background work")
	require.NoError(t, service.Start(context.Background()))
	require.NotNil(t, service.cancel)
	require.NoError(t, service.Start(context.Background()), "Start must be idempotent")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, service.Shutdown(ctx))
	require.Nil(t, service.cancel)
	require.NoError(t, service.Shutdown(ctx), "Shutdown must be idempotent")
}

func TestPromptServiceStartReportsDependencyFailureWithoutPanic(t *testing.T) {
	service := &PromptService{}
	require.Error(t, service.Start(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, service.Shutdown(ctx))
}

func TestPromptServiceBlockingLatestTurnOnlyUsesNarrowSnapshot(t *testing.T) {
	seen := make([]string, 0, 2)
	evaluator := newGuardEvaluator(PromptScannerFunc(func(_ context.Context, _ ActiveEndpoint, chunk string, _ []string) (*NormalizedResult, error) {
		seen = append(seen, chunk)
		return &NormalizedResult{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, ScannerScores: map[string]float64{}, ScannerEvidence: map[string]string{}}, nil
	}), nil, NewAtomicMetrics(), 2, 2)
	service := &PromptService{
		config: &fakeConfigStore{active: true, cfg: ActiveConfig{
			RiskControlEnabled: true, Enabled: true, BlockingEnabled: true, BlockingLatestTurnOnly: true, AllGroups: true,
			Scanners: AllScannerIDs, Endpoints: []ActiveEndpoint{{ID: "guard-1", Enabled: true, TimeoutMS: 1000, InputLimit: 4096}},
		}},
		evaluator: evaluator,
	}
	request := asyncRequest()
	request.Body = []byte(`{"messages":[{"role":"system","content":"system instruction"},{"role":"user","content":"older user input"},{"role":"assistant","content":"previous output"},{"role":"user","content":"latest user input"}]}`)
	decision, err := service.Evaluate(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, DecisionAllow, decision.Kind)
	require.Equal(t, []string{"latest user input", "previous output"}, seen)
}

func TestEndpointNeedsProbeTriggerMatrix(t *testing.T) {
	auto := ResponseModeAuto
	jsonObject := ResponseModeJSONObject
	maxChanged := DefaultMaxOutputTokens + 64
	old := ActiveEndpoint{ID: "one", Enabled: true, BaseURL: "https://review.example.com", Model: "reviewer", ResponseMode: ResponseModeAuto, MaxOutputTokens: DefaultMaxOutputTokens}
	base := UpdateEndpoint{ID: "one", Enabled: true, BaseURL: "https://review.example.com/v1", Model: old.Model, ResponseMode: &auto, MaxOutputTokens: func() *int { value := DefaultMaxOutputTokens; return &value }()}
	tests := []struct {
		name                   string
		currentEnabled, nextOn bool
		next                   UpdateEndpoint
		hadOld, want           bool
	}{
		{name: "unchanged", currentEnabled: true, nextOn: true, next: base, hadOld: true},
		{name: "pure global disable", currentEnabled: true, nextOn: false, next: base, hadOld: true},
		{name: "global off to on", currentEnabled: false, nextOn: true, next: base, hadOld: true, want: true},
		{name: "new endpoint", currentEnabled: true, nextOn: true, next: base, hadOld: false, want: true},
		{name: "new disabled endpoint while global off", currentEnabled: false, nextOn: false, next: func() UpdateEndpoint { value := base; value.Enabled = false; return value }(), hadOld: false, want: true},
		{name: "newly enabled", currentEnabled: true, nextOn: true, next: base, hadOld: true, want: true},
		{name: "base URL", currentEnabled: true, nextOn: true, next: func() UpdateEndpoint { value := base; value.BaseURL = "https://other.example.com/v1"; return value }(), hadOld: true, want: true},
		{name: "base URL while global off", currentEnabled: false, nextOn: false, next: func() UpdateEndpoint {
			value := base
			value.Enabled = false
			value.BaseURL = "https://other.example.com/v1"
			return value
		}(), hadOld: true, want: true},
		{name: "model", currentEnabled: true, nextOn: true, next: func() UpdateEndpoint { value := base; value.Model = "other-reviewer"; return value }(), hadOld: true, want: true},
		{name: "token", currentEnabled: true, nextOn: true, next: func() UpdateEndpoint { value := base; value.Token = "replacement"; return value }(), hadOld: true, want: true},
		{name: "clear token while active", currentEnabled: true, nextOn: true, next: func() UpdateEndpoint { value := base; value.ClearToken = true; return value }(), hadOld: true, want: true},
		{name: "clear token while disabling globally", currentEnabled: true, nextOn: false, next: func() UpdateEndpoint { value := base; value.ClearToken = true; return value }(), hadOld: true, want: false},
		{name: "clear token while disabling endpoint", currentEnabled: true, nextOn: true, next: func() UpdateEndpoint { value := base; value.Enabled = false; value.ClearToken = true; return value }(), hadOld: true, want: false},
		{name: "response mode", currentEnabled: true, nextOn: true, next: func() UpdateEndpoint { value := base; value.ResponseMode = &jsonObject; return value }(), hadOld: true, want: true},
		{name: "max output only", currentEnabled: true, nextOn: true, next: func() UpdateEndpoint { value := base; value.MaxOutputTokens = &maxChanged; return value }(), hadOld: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidateOld := old
			if tt.name == "newly enabled" {
				candidateOld.Enabled = false
			}
			require.Equal(t, tt.want, endpointNeedsProbe(tt.currentEnabled, tt.nextOn, tt.next, candidateOld, tt.hadOld))
		})
	}
	legacyDisabled := old
	legacyDisabled.Enabled = false
	legacyDisabled.RequiresReconfigure = true
	unchangedDisabled := base
	unchangedDisabled.Enabled = false
	require.False(t, endpointNeedsProbe(false, false, unchangedDisabled, legacyDisabled, true), "unchanged retired endpoint must stay dormant while disabled")
	reconfiguredDisabled := unchangedDisabled
	reconfiguredDisabled.Model = "replacement-reviewer"
	require.True(t, endpointNeedsProbe(false, false, reconfiguredDisabled, legacyDisabled, true), "editing a retired disabled endpoint must run the real probe")
	ordinaryDisabled := legacyDisabled
	ordinaryDisabled.RequiresReconfigure = false
	require.False(t, endpointNeedsProbe(false, false, unchangedDisabled, ordinaryDisabled, true), "ordinary unchanged disabled endpoint stays dormant")
	invalidToken := ordinaryDisabled
	invalidToken.TokenInvalid = true
	enableInvalidToken := unchangedDisabled
	enableInvalidToken.Enabled = true
	require.True(t, endpointNeedsProbe(false, false, enableInvalidToken, invalidToken, true), "enabling an endpoint with an invalid stored token must not skip probe")
}

func TestSaveConfigRequiresReplacementForInvalidStoredToken(t *testing.T) {
	auto := ResponseModeAuto
	store := &fakeConfigStore{active: true, cfg: ActiveConfig{Endpoints: []ActiveEndpoint{{
		ID: "one", BaseURL: "https://review.example.com", Model: "reviewer", ResponseMode: ResponseModeAuto,
		MaxOutputTokens: DefaultMaxOutputTokens, TokenInvalid: true,
	}}}}
	service := &PromptService{config: store, scanner: NewOpenAICompatibleScanner(), clock: realClock{}, probes: map[string]ProbeResult{}}
	_, err := service.SaveConfig(context.Background(), UpdateConfigRequest{Endpoints: []UpdateEndpoint{{
		ID: "one", Name: "One", BaseURL: "https://review.example.com", Model: "reviewer", ResponseMode: &auto,
		TimeoutMS: 1000, InputLimit: 1000, Enabled: true,
	}}}, 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "prompt_audit_endpoint_token_replacement_required")
}

func TestPromptServiceRejectsInvalidDeleteConfirmationClaims(t *testing.T) {
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	start, end := now.Add(-time.Hour), now.Add(time.Hour)
	filter := EventFilter{Decision: string(EventCritical), StartAt: &start, EndAt: &end}
	const snapshotMaxID int64 = 10
	filterHash := FilterHash(filter, snapshotMaxID)
	validClaims := deleteClaims{
		FilterHash: filterHash, SnapshotMaxID: snapshotMaxID, AdminID: 7,
		IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute),
	}
	claimsToken := func(claims deleteClaims) string {
		raw, err := json.Marshal(claims)
		require.NoError(t, err)
		return string(raw)
	}
	validRequest := DeleteByFilterRequest{
		Filter: filter, SnapshotMaxID: snapshotMaxID, FilterHash: filterHash,
		ConfirmationToken: claimsToken(validClaims), Confirm: true,
	}

	tests := []struct {
		name    string
		request DeleteByFilterRequest
		adminID int64
	}{
		{name: "confirm false", request: func() DeleteByFilterRequest { value := validRequest; value.Confirm = false; return value }(), adminID: 7},
		{name: "malformed token", request: func() DeleteByFilterRequest {
			value := validRequest
			value.ConfirmationToken = "not-json"
			return value
		}(), adminID: 7},
		{name: "different administrator", request: validRequest, adminID: 8},
		{name: "filter hash mismatch", request: func() DeleteByFilterRequest {
			value := validRequest
			value.FilterHash = strings.Repeat("b", 64)
			return value
		}(), adminID: 7},
		{name: "snapshot mismatch", request: func() DeleteByFilterRequest { value := validRequest; value.SnapshotMaxID++; return value }(), adminID: 7},
		{name: "expired", request: func() DeleteByFilterRequest {
			value := validRequest
			claims := validClaims
			claims.ExpiresAt = now
			value.ConfirmationToken = claimsToken(claims)
			return value
		}(), adminID: 7},
	}

	service := &PromptService{config: &fakeConfigStore{}, clock: fixedClock{now: now}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := service.DeleteByFilter(context.Background(), test.request, test.adminID)
			require.Error(t, err)
			require.Nil(t, result)
		})
	}
}
