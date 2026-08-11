package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokVoiceBinding struct {
	accountID int64
	ttl       time.Duration
}

type grokVoiceBindingCacheStub struct {
	GatewayCache
	bindings           map[string]grokVoiceBinding
	groupID            int64
	key                string
	accountID          int64
	ttl                time.Duration
	claimCalls         int
	setCalls           int
	releaseCanceled    bool
	releaseHasDeadline bool
	commitCanceled     bool
	commitHasDeadline  bool
}

func (c *grokVoiceBindingCacheStub) GetSessionAccountID(_ context.Context, groupID int64, key string) (int64, error) {
	binding, ok := c.bindings[c.bindingKey(groupID, key)]
	if !ok {
		return 0, ErrStickySessionNotFound
	}
	return binding.accountID, nil
}

func (c *grokVoiceBindingCacheStub) SetSessionAccountID(_ context.Context, groupID int64, key string, accountID int64, ttl time.Duration) error {
	c.setCalls++
	if c.bindings == nil {
		c.bindings = make(map[string]grokVoiceBinding)
	}
	c.bindings[c.bindingKey(groupID, key)] = grokVoiceBinding{accountID: accountID, ttl: ttl}
	c.groupID = groupID
	c.key = key
	c.accountID = accountID
	c.ttl = ttl
	return nil
}

func (c *grokVoiceBindingCacheStub) ClaimSessionAccountIDWithTTL(_ context.Context, groupID int64, key string, accountID int64, ttl time.Duration) (int64, error) {
	c.claimCalls++
	if c.bindings == nil {
		c.bindings = make(map[string]grokVoiceBinding)
	}
	bindingKey := c.bindingKey(groupID, key)
	if binding, ok := c.bindings[bindingKey]; ok && binding.accountID != accountID {
		return binding.accountID, nil
	}
	c.bindings[bindingKey] = grokVoiceBinding{accountID: accountID, ttl: ttl}
	c.groupID = groupID
	c.key = key
	c.accountID = accountID
	c.ttl = ttl
	return accountID, nil
}

func (c *grokVoiceBindingCacheStub) bindingKey(groupID int64, key string) string {
	return strings.Join([]string{strings.TrimSpace(key), strconv.FormatInt(groupID, 10)}, "|")
}

func (c *grokVoiceBindingCacheStub) ReserveGrokVoiceLibrary(context.Context, int64, string, int64, string, time.Duration) (bool, error) {
	return true, nil
}

func (c *grokVoiceBindingCacheStub) CommitGrokVoiceLibraryReservation(ctx context.Context, groupID int64, libraryKey, resourceKey string, accountID int64, _ string) error {
	c.commitCanceled = ctx.Err() != nil
	_, c.commitHasDeadline = ctx.Deadline()
	if err := c.SetSessionAccountID(ctx, groupID, libraryKey, accountID, 0); err != nil {
		return err
	}
	return c.SetSessionAccountID(ctx, groupID, resourceKey, accountID, 0)
}

func (c *grokVoiceBindingCacheStub) ReleaseGrokVoiceLibraryReservation(ctx context.Context, _ int64, _ string, _ int64, _ string) error {
	c.releaseCanceled = ctx.Err() != nil
	_, c.releaseHasDeadline = ctx.Deadline()
	return nil
}

func TestBuildGrokVoiceURL_UsesAPIDefaultForCLIProxyBase(t *testing.T) {
	account := &Account{
		Platform: PlatformGrok,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"base_url": xai.DefaultCLIBaseURL,
		},
	}
	url, err := buildGrokVoiceURL(account, nil, "tts")
	require.NoError(t, err)
	require.Equal(t, xai.DefaultBaseURL+"/tts", url)

	url, err = buildGrokVoiceURL(account, nil, "realtime")
	require.NoError(t, err)
	require.Equal(t, xai.DefaultBaseURL+"/realtime", url)
}

func TestBuildGrokVoiceURL_EmptyBaseFallsBackToAPI(t *testing.T) {
	account := &Account{
		Platform:    PlatformGrok,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{},
	}
	url, err := buildGrokVoiceURL(account, nil, "stt")
	require.NoError(t, err)
	require.Equal(t, xai.DefaultBaseURL+"/stt", url)
}

func TestBuildGrokVoiceURL_RequiresEndpoint(t *testing.T) {
	account := &Account{Platform: PlatformGrok, Type: AccountTypeOAuth}
	_, err := buildGrokVoiceURL(account, nil, "  ")
	require.Error(t, err)
}

func TestBuildGrokVoiceURL_EncodesCustomVoicePathSegments(t *testing.T) {
	account := &Account{Platform: PlatformGrok, Type: AccountTypeOAuth}
	got, err := buildGrokVoiceURL(account, nil, "custom-voices/nlbqfwie/audio")
	require.NoError(t, err)
	require.Equal(t, xai.DefaultBaseURL+"/custom-voices/nlbqfwie/audio", got)

	_, err = buildGrokVoiceURL(account, nil, "custom-voices/../audio")
	require.Error(t, err)
}

func TestForwardGrokVoice_RejectsNonGrok(t *testing.T) {
	svc := &OpenAIGatewayService{}
	_, err := svc.ForwardGrokVoice(context.Background(), nil, &Account{Platform: PlatformOpenAI}, "tts", []byte(`{}`), "application/json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "not supported")
}

func TestForwardGrokVoice_RejectsUnknownEndpoint(t *testing.T) {
	svc := &OpenAIGatewayService{}
	_, err := svc.ForwardGrokVoice(context.Background(), nil, &Account{Platform: PlatformGrok}, "unknown", []byte(`{}`), "application/json")
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported")
}

func TestGrokVoiceHTTPContext_DetachesCancellationAndBoundsLifetime(t *testing.T) {
	type contextKey string
	const key contextKey = "voice-value"
	parent, cancelParent := context.WithCancel(context.WithValue(context.Background(), key, "preserved"))
	cfg := &config.Config{}
	cfg.Gateway.Grok.VoiceHTTPTimeoutSeconds = 1

	ctx, cancel := grokVoiceHTTPContext(parent, cfg)
	defer cancel()
	cancelParent()

	require.Equal(t, "preserved", ctx.Value(key))
	require.NoError(t, ctx.Err(), "client cancellation must remain detached")
	deadline, ok := ctx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(time.Second), deadline, 100*time.Millisecond)

	defaultCtx, defaultCancel := grokVoiceHTTPContext(context.Background(), nil)
	defer defaultCancel()
	defaultDeadline, ok := defaultCtx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(time.Duration(config.DefaultGrokVoiceHTTPTimeoutSeconds)*time.Second), defaultDeadline, 100*time.Millisecond)
}

func TestGrokVoiceHTTPTimeout_LeavesCustomVoiceOwnershipCommitBudget(t *testing.T) {
	maxRequestLifetime := time.Duration(config.MaxGrokVoiceHTTPTimeoutSeconds) * time.Second
	require.Less(t, maxRequestLifetime+grokVoiceOwnershipMutationTimeout, grokVoiceLibraryReservationTTL)
}

func TestBuildGrokVoiceHTTPRequestURL_ForwardsOnlyListPagination(t *testing.T) {
	inbound := url.Values{
		"limit":            {"50"},
		"pagination_token": {"a+b/="},
		"ignored":          {"secret"},
	}
	got, err := buildGrokVoiceHTTPRequestURL(
		"https://api.x.ai/v1/custom-voices?existing=1",
		"custom-voices",
		http.MethodGet,
		inbound,
	)
	require.NoError(t, err)
	parsed, err := url.Parse(got)
	require.NoError(t, err)
	require.Equal(t, "1", parsed.Query().Get("existing"))
	require.Equal(t, "50", parsed.Query().Get("limit"))
	require.Equal(t, "a+b/=", parsed.Query().Get("pagination_token"))
	require.Empty(t, parsed.Query().Get("ignored"))

	for _, tc := range []struct {
		name     string
		endpoint string
		method   string
	}{
		{name: "create", endpoint: "custom-voices", method: http.MethodPost},
		{name: "get by id", endpoint: "custom-voices/voice-1", method: http.MethodGet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildGrokVoiceHTTPRequestURL("https://api.x.ai/v1/"+tc.endpoint, tc.endpoint, tc.method, inbound)
			require.NoError(t, err)
			parsed, err := url.Parse(got)
			require.NoError(t, err)
			require.Empty(t, parsed.RawQuery)
		})
	}
}

func TestBuildGrokRealtimeWebSocketURL_MapsSchemeAndFiltersQuery(t *testing.T) {
	inbound := url.Values{
		"conversation_id":  {" conv+a/b= "},
		"call_id":          {"call+a/b="},
		"reasoning.effort": {"low"},
		"ignored":          {"secret"},
		"model":            {"client-alias"},
	}
	for _, tc := range []struct {
		base       string
		wantScheme string
	}{
		{base: "https://api.x.ai/v1/realtime?existing=1&call_id=base-call", wantScheme: "wss"},
		{base: "http://relay.local/v1/realtime", wantScheme: "ws"},
		{base: "wss://relay.local/v1/realtime", wantScheme: "wss"},
		{base: "ws://relay.local/v1/realtime", wantScheme: "ws"},
	} {
		t.Run(tc.base, func(t *testing.T) {
			got, err := buildGrokRealtimeWebSocketURL(tc.base, "mapped-model", inbound)
			require.NoError(t, err)
			parsed, err := url.Parse(got)
			require.NoError(t, err)
			require.Equal(t, tc.wantScheme, parsed.Scheme)
			require.Equal(t, "mapped-model", parsed.Query().Get("model"))
			require.Equal(t, "conv+a/b=", parsed.Query().Get("conversation_id"))
			require.Empty(t, parsed.Query().Get("call_id"), "untrusted call identifiers must not reach xAI")
			if parsed.Query().Has("existing") {
				require.Equal(t, "1", parsed.Query().Get("existing"))
			}
			require.Equal(t, "low", parsed.Query().Get("reasoning.effort"))
			require.Empty(t, parsed.Query().Get("ignored"))
		})
	}

	_, err := buildGrokRealtimeWebSocketURL("ftp://relay.local/realtime", "mapped-model", inbound)
	require.ErrorContains(t, err, "unsupported grok realtime URL scheme")
}

func TestGrokCustomVoiceLibraryBinding_IsStableIsolatedAndPersistent(t *testing.T) {
	require.Empty(t, GrokCustomVoiceLibrarySessionHash(0))
	first := GrokCustomVoiceLibrarySessionHash(10)
	require.NotEmpty(t, first)
	require.Equal(t, first, GrokCustomVoiceLibrarySessionHash(10))
	require.NotEqual(t, first, GrokCustomVoiceLibrarySessionHash(11))

	cache := &grokVoiceBindingCacheStub{bindings: make(map[string]grokVoiceBinding)}
	svc := &OpenAIGatewayService{cache: cache}
	groupID := int64(7)
	err := svc.BindGrokCustomVoiceLibraryAccount(context.Background(), &groupID, 10, 30)
	require.NoError(t, err)
	require.Equal(t, groupID, cache.groupID)
	require.Contains(t, cache.key, "openai:grok-custom-voice-library:")
	require.Equal(t, int64(30), cache.accountID)
	require.Zero(t, cache.ttl, "custom voice affinity must not expire")

	accountID, err := svc.ResolveGrokCustomVoiceLibraryAccount(context.Background(), &groupID, 10)
	require.NoError(t, err)
	require.Equal(t, int64(30), accountID)

	otherGroup := int64(8)
	_, err = svc.ResolveGrokCustomVoiceLibraryAccount(context.Background(), &otherGroup, 10)
	require.ErrorIs(t, err, ErrStickySessionNotFound)
}

func TestGrokRealtimeResumeBinding_IsolatedValidatedAndExpiring(t *testing.T) {
	cache := &grokVoiceBindingCacheStub{bindings: make(map[string]grokVoiceBinding)}
	svc := &OpenAIGatewayService{cache: cache}
	groupID := int64(7)

	require.NoError(t, svc.BindGrokRealtimeConversationAccount(context.Background(), &groupID, 10, "conv-1", 30))
	require.Equal(t, grokRealtimeConversationBindingTTL, cache.ttl)
	require.NotZero(t, cache.ttl)
	require.Equal(t, 1, cache.claimCalls)
	require.Zero(t, cache.setCalls, "conversation TTL must be part of the atomic claim")

	accountID, sessionHash, err := svc.ResolveGrokRealtimeResumeAccount(context.Background(), &groupID, 10, "conv-1")
	require.NoError(t, err)
	require.Equal(t, int64(30), accountID)
	require.Equal(t, GrokRealtimeConversationSessionHash(10, "conv-1"), sessionHash)

	_, _, err = svc.ResolveGrokRealtimeResumeAccount(context.Background(), &groupID, 11, "conv-1")
	require.ErrorIs(t, err, ErrGrokRealtimeConversationNotOwned)
	_, _, err = svc.ResolveGrokRealtimeResumeAccount(context.Background(), &groupID, 10, "")
	require.ErrorIs(t, err, ErrGrokRealtimeConversationNotOwned)
}

func TestGrokRealtimeRouting_FreshUsesOwnedCustomVoiceLibrary(t *testing.T) {
	cache := &grokVoiceBindingCacheStub{bindings: make(map[string]grokVoiceBinding)}
	svc := &OpenAIGatewayService{cache: cache}
	groupID := int64(7)

	accountID, sessionHash, library, err := svc.ResolveGrokRealtimeRoutingAccount(
		context.Background(), &groupID, 10, "",
	)
	require.NoError(t, err)
	require.Zero(t, accountID)
	require.Empty(t, sessionHash)
	require.False(t, library)

	require.NoError(t, svc.BindGrokCustomVoiceLibraryAccount(context.Background(), &groupID, 10, 30))
	accountID, sessionHash, library, err = svc.ResolveGrokRealtimeRoutingAccount(
		context.Background(), &groupID, 10, "",
	)
	require.NoError(t, err)
	require.Equal(t, int64(30), accountID)
	require.Equal(t, GrokCustomVoiceLibrarySessionHash(10), sessionHash)
	require.True(t, library)
	require.True(t, grokVoiceHardAccountAffinityFromContext(WithGrokVoiceHardAccountAffinity(context.Background())))

	// Explicit resume never falls back to the Custom Voice library.
	_, _, library, err = svc.ResolveGrokRealtimeRoutingAccount(
		context.Background(), &groupID, 10, "missing-conversation",
	)
	require.ErrorIs(t, err, ErrGrokRealtimeConversationNotOwned)
	require.False(t, library)
}

func TestExtractGrokRealtimeConversationIDs_OnlyConversationCreated(t *testing.T) {
	require.Equal(t, []string{"conv-1"}, ExtractGrokRealtimeConversationIDs(
		[]byte(`{"type":"conversation.created","conversation":{"id":"conv-1"},"call_id":"must-not-bind"}`),
	))
	require.Empty(t, ExtractGrokRealtimeConversationIDs(
		[]byte(`{"type":"response.function_call_arguments.done","conversation":{"id":"conv-2"},"conversation_id":"conv-2","call_id":"call-2"}`),
	))
}

func TestForwardGrokVoice_CustomVoiceRequiresValidOwnerSignature(t *testing.T) {
	gin.SetMode(gin.TestMode)
	account := &Account{ID: 30, Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "secret"}}
	svc := &OpenAIGatewayService{}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	_, err := svc.ForwardGrokVoice(context.Background(), c, account, "custom-voices", []byte(`{}`), "application/json")
	require.ErrorContains(t, err, "owner is required")
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.JSONEq(t, `{"error":{"type":"api_error","message":"custom voice owner is required"}}`, recorder.Body.String())

	recorder = httptest.NewRecorder()
	c, _ = gin.CreateTestContext(recorder)
	_, err = svc.ForwardGrokVoice(context.Background(), c, account, "custom-voices", []byte(`{}`), "application/json", GrokVoiceRequestOwner{})
	require.ErrorContains(t, err, "owner is invalid")
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"type":"api_error"`)

	owner, err := resolveGrokVoiceRequestOwner([]GrokVoiceRequestOwner{{UserID: 10}})
	require.NoError(t, err)
	require.NotNil(t, owner)
	require.Nil(t, owner.GroupID)
}

func TestGrokCustomVoiceOwnership_IsolatesUsersAcrossListResourcesAndTTS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &grokVoiceBindingCacheStub{bindings: make(map[string]grokVoiceBinding)}
	svc := &OpenAIGatewayService{cache: cache}
	groupID := int64(7)
	accountID := int64(30)
	userOneVoice := "abcd1234"
	userTwoVoice := "efgh5678"
	require.NoError(t, svc.BindGrokCustomVoiceResourceAccount(context.Background(), &groupID, 10, userOneVoice, accountID))
	require.NoError(t, svc.BindGrokCustomVoiceResourceAccount(context.Background(), &groupID, 11, userTwoVoice, accountID))

	upstreamList := []byte(`{"voices":[{"voice_id":"abcd1234","name":"one"},{"voice_id":"efgh5678","name":"two"}],"pagination_token":null}`)
	filtered, err := svc.filterGrokCustomVoiceList(context.Background(), GrokVoiceRequestOwner{GroupID: &groupID, UserID: 11}, accountID, upstreamList)
	require.NoError(t, err)
	require.JSONEq(t, `{"voices":[{"voice_id":"efgh5678","name":"two"}],"pagination_token":null}`, string(filtered))

	account := &Account{ID: accountID, Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "secret"}}
	owner := GrokVoiceRequestOwner{GroupID: &groupID, UserID: 11}
	for _, tc := range []struct {
		name     string
		method   string
		endpoint string
		body     []byte
	}{
		{name: "get", method: http.MethodGet, endpoint: "custom-voices/" + userOneVoice},
		{name: "audio", method: http.MethodGet, endpoint: "custom-voices/" + userOneVoice + "/audio"},
		{name: "patch", method: http.MethodPatch, endpoint: "custom-voices/" + userOneVoice, body: []byte(`{"name":"stolen"}`)},
		{name: "delete", method: http.MethodDelete, endpoint: "custom-voices/" + userOneVoice},
		{name: "tts", method: http.MethodPost, endpoint: "tts", body: []byte(`{"input":"hello","voice_id":"abcd1234"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(tc.method, "/v1/"+tc.endpoint, nil)
			_, err := svc.ForwardGrokVoice(context.Background(), c, account, tc.endpoint, tc.body, "application/json", owner)
			require.ErrorIs(t, err, ErrGrokCustomVoiceNotOwned)
			require.Equal(t, http.StatusNotFound, recorder.Code)
			require.Contains(t, recorder.Body.String(), `"type":"not_found_error"`)
		})
	}
}

func TestGrokCustomVoiceReservationCleanup_UsesIndependentTimeout(t *testing.T) {
	cache := &grokVoiceBindingCacheStub{bindings: make(map[string]grokVoiceBinding)}
	svc := &OpenAIGatewayService{cache: cache}
	groupID := int64(7)
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	err := svc.ReleaseGrokCustomVoiceLibraryReservation(canceledCtx, &groupID, 10, 30, "reservation-token")
	require.NoError(t, err)
	require.False(t, cache.releaseCanceled)
	require.True(t, cache.releaseHasDeadline)
}

func TestForwardGrokVoice_CreateCommitsOwnershipAfterClientCancellation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &grokVoiceBindingCacheStub{bindings: make(map[string]grokVoiceBinding)}
	svc := &OpenAIGatewayService{
		cache: cache,
		httpUpstream: &httpUpstreamStub{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"voice_id":"abcd1234"}`)),
		}},
	}
	account := &Account{ID: 30, Platform: PlatformGrok, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "secret"}}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	requestCtx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/custom-voices", nil).WithContext(requestCtx)
	cancel()
	groupID := int64(7)

	_, err := svc.ForwardGrokVoice(requestCtx, c, account, "custom-voices", []byte(`{}`), "application/json", GrokVoiceRequestOwner{
		GroupID: &groupID, UserID: 10, LibraryReservationToken: "reservation-token",
	})
	require.NoError(t, err)
	require.False(t, cache.commitCanceled)
	require.True(t, cache.commitHasDeadline)
	require.Equal(t, http.StatusOK, recorder.Code)
	boundAccount, err := svc.ResolveGrokCustomVoiceResourceAccount(context.Background(), &groupID, 10, "abcd1234")
	require.NoError(t, err)
	require.Equal(t, int64(30), boundAccount)
}
