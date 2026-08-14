package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

// supportedGrokVoiceHTTPEndpoints are xAI Voice HTTP paths we forward as-is.
var supportedGrokVoiceHTTPEndpoints = map[string]struct{}{
	"tts":           {},
	"stt":           {},
	"custom-voices": {},
}

// ForwardGrokVoice forwards the official xAI Voice HTTP APIs (/tts, /stt, and
// the custom-voices CRUD/audio subresources).
// The response is intentionally passed through because TTS returns audio bytes
// while STT returns JSON and xAI may add format-specific headers.
func (s *OpenAIGatewayService) ForwardGrokVoice(ctx context.Context, c *gin.Context, account *Account, endpoint string, body []byte, contentType string, owners ...GrokVoiceRequestOwner) (*OpenAIForwardResult, error) {
	if s == nil || account == nil {
		return nil, fmt.Errorf("grok voice service/account is required")
	}
	if account.Platform != PlatformGrok {
		return nil, fmt.Errorf("account platform %s is not supported for grok voice", account.Platform)
	}
	endpoint = strings.Trim(strings.TrimSpace(endpoint), "/")
	parts := strings.Split(endpoint, "/")
	baseEndpoint := parts[0]
	if _, ok := supportedGrokVoiceHTTPEndpoints[baseEndpoint]; !ok {
		return nil, fmt.Errorf("unsupported grok voice endpoint: %s", endpoint)
	}
	if len(parts) > 1 && baseEndpoint != "custom-voices" {
		return nil, fmt.Errorf("unsupported grok voice endpoint: %s", endpoint)
	}
	if baseEndpoint == "custom-voices" {
		if len(parts) > 3 || (len(parts) == 3 && parts[2] != "audio") {
			return nil, fmt.Errorf("unsupported grok voice endpoint: %s", endpoint)
		}
	}
	for _, part := range parts[1:] {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "?#\\") {
			return nil, fmt.Errorf("invalid grok voice endpoint path")
		}
	}
	owner, err := resolveGrokVoiceRequestOwner(owners)
	if err != nil {
		writeGrokVoiceOwnershipError(c, http.StatusServiceUnavailable, err.Error())
		return nil, err
	}
	if baseEndpoint == "custom-voices" && owner == nil {
		err = fmt.Errorf("custom voice owner is required")
		writeGrokVoiceOwnershipError(c, http.StatusServiceUnavailable, err.Error())
		return nil, err
	}
	if owner != nil {
		if err = s.authorizeGrokVoicePayload(ctx, *owner, account.ID, baseEndpoint, endpoint, body); err != nil {
			writeGrokVoiceOwnershipError(c, grokVoiceOwnershipHTTPStatus(err), "Custom Voice is not available")
			return nil, err
		}
	}
	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, err
	}
	targetURL, err := buildGrokVoiceURL(account, s.cfg, endpoint)
	if err != nil {
		return nil, err
	}
	upstreamCtx, release := grokVoiceHTTPContext(ctx, s.cfg)
	defer release()
	method := http.MethodPost
	if c != nil && c.Request != nil && strings.TrimSpace(c.Request.Method) != "" {
		method = c.Request.Method
	}
	targetURL, err = buildGrokVoiceHTTPRequestURL(targetURL, endpoint, method, grokVoiceInboundQuery(c))
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(upstreamCtx, method, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json, audio/*")
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/json"
	}
	req.Header.Set("Content-Type", contentType)
	// Match media path: CLI identity headers only on the CLI chat proxy.
	// Official api.x.ai voice rejects or mistreats OAuth when CLI headers are stamped.
	if account.IsGrokOAuth() && isGrokCLIProxyTarget(targetURL) {
		applyGrokCLIHeaders(req.Header)
	}
	account.ApplyHeaderOverrides(req.Header)

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	started := time.Now()
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(started).Milliseconds())
	if err != nil {
		return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return s.handleGrokMediaErrorResponse(ctx, resp, c, account, resp.Header.Get("x-request-id"), endpoint)
	}
	data, readErr := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	duration := time.Since(started)
	upstreamID := firstNonEmpty(resp.Header.Get("x-request-id"), resp.Header.Get("xai-request-id"))
	result := &OpenAIForwardResult{
		// Forced durable money-event id so usage_billing_dedup cannot collapse under a reused client id.
		RequestID:     StableGrokAudioBillingRequestID(upstreamID),
		Model:         baseEndpoint,
		UpstreamModel: baseEndpoint,
		Duration:      duration,
		AudioUsage:    estimateGrokVoiceAudioUsage(baseEndpoint, body, contentType, data, duration),
	}
	if readErr != nil {
		// A 2xx response means the paid upstream operation already happened. Return
		// its usage even when the downstream body cannot be completed.
		if c != nil && c.Writer != nil && !c.Writer.Written() {
			MarkResponseCommitted(c)
			writeGrokMediaErrorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream response could not be read")
		}
		return result, &grokVoicePostSuccessError{err: readErr}
	}
	if owner != nil {
		ownershipCtx, cancel := context.WithTimeout(context.Background(), grokVoiceOwnershipMutationTimeout)
		data, err = s.applyGrokVoiceOwnershipToResponse(ownershipCtx, *owner, account.ID, baseEndpoint, endpoint, method, body, data)
		cancel()
		if err != nil {
			writeGrokVoiceOwnershipError(c, http.StatusServiceUnavailable, "Custom Voice ownership state unavailable")
			return result, &grokVoicePostSuccessError{err: err}
		}
	}
	writeGrokMediaResponse(c, resp, data, s.responseHeaderFilter)
	return result, nil
}

func grokVoiceHTTPContext(ctx context.Context, cfg *config.Config) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	} else {
		ctx = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(ctx, resolveGrokVoiceHTTPTimeout(cfg))
}

func resolveGrokVoiceHTTPTimeout(cfg *config.Config) time.Duration {
	seconds := config.DefaultGrokVoiceHTTPTimeoutSeconds
	if cfg != nil && cfg.Gateway.Grok.VoiceHTTPTimeoutSeconds > 0 {
		seconds = cfg.Gateway.Grok.VoiceHTTPTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

// grokVoicePostSuccessError marks a non-retryable failure after xAI has
// accepted and completed the paid operation. Callers must bill a non-nil
// result before handling this error and must never fail over to another account.
type grokVoicePostSuccessError struct {
	err error
}

func (e *grokVoicePostSuccessError) Error() string {
	if e == nil || e.err == nil {
		return "grok voice post-success processing failed"
	}
	return e.err.Error()
}

func (e *grokVoicePostSuccessError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// ProxyGrokRealtime relays JSON Realtime events to xAI's native Voice WS.
// Audio is carried as base64 inside JSON events, so preserving the JSON bytes
// is sufficient and avoids translating protocol event types.
func (s *OpenAIGatewayService) ProxyGrokRealtime(ctx context.Context, c *gin.Context, client *coderws.Conn, account *Account, token, model string, owners ...GrokVoiceRequestOwner) (time.Duration, bool, error) {
	if s == nil || client == nil || account == nil {
		return 0, false, fmt.Errorf("realtime service, client, and account are required")
	}
	if account.Platform != PlatformGrok {
		return 0, false, fmt.Errorf("account platform %s is not supported for grok realtime", account.Platform)
	}
	owner, err := resolveGrokVoiceRequestOwner(owners)
	if err != nil {
		return 0, false, err
	}
	base, err := buildGrokVoiceURL(account, s.cfg, "realtime")
	if err != nil {
		return 0, false, err
	}
	upstreamURL, err := buildGrokRealtimeWebSocketURL(base, model, grokVoiceInboundQuery(c))
	if err != nil {
		return 0, false, err
	}
	headers := http.Header{"Authorization": []string{"Bearer " + token}}
	// Match media/voice HTTP: CLI headers only on CLI proxy hosts.
	if account.IsGrokOAuth() && isGrokCLIProxyTarget(upstreamURL) {
		applyGrokCLIHeaders(headers)
	}
	if account != nil {
		account.ApplyHeaderOverrides(headers)
	}

	dialer := s.getOpenAIWSPassthroughDialer()
	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	upstream, _, _, err := dialer.Dial(ctx, upstreamURL, headers, proxyURL)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = upstream.Close() }()
	relayStarted := time.Now()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	errCh := make(chan error, 2)
	var audioObserved atomic.Bool

	// Upstream → client
	go func() {
		for {
			msg, readErr := upstream.ReadMessage(ctx)
			if readErr != nil {
				errCh <- readErr
				return
			}
			if grokRealtimeEventHasAudio(msg) {
				audioObserved.Store(true)
			}
			if owner != nil {
				for _, conversationID := range ExtractGrokRealtimeConversationIDs(msg) {
					if bindErr := s.BindGrokRealtimeConversationAccount(ctx, owner.GroupID, owner.UserID, conversationID, account.ID); bindErr != nil {
						errCh <- fmt.Errorf("bind grok realtime conversation: %w", bindErr)
						return
					}
				}
			}
			if writeErr := client.Write(ctx, coderws.MessageText, msg); writeErr != nil {
				errCh <- writeErr
				return
			}
		}
	}()

	// Client → upstream (JSON events only)
	go func() {
		for {
			kind, msg, readErr := client.Read(ctx)
			if readErr != nil {
				errCh <- readErr
				return
			}
			if kind != coderws.MessageText && kind != coderws.MessageBinary {
				continue
			}
			if grokRealtimeEventHasAudio(msg) {
				audioObserved.Store(true)
			}
			var raw json.RawMessage
			if unmarshalErr := json.Unmarshal(msg, &raw); unmarshalErr != nil {
				errCh <- fmt.Errorf("invalid realtime event: %w", unmarshalErr)
				return
			}
			if owner != nil {
				if authorizeErr := s.authorizeGrokVoicePayload(ctx, *owner, account.ID, "realtime", "realtime", msg); authorizeErr != nil {
					errCh <- fmt.Errorf("custom voice is not available: %w", authorizeErr)
					return
				}
			}
			if writeErr := upstream.WriteJSON(ctx, raw); writeErr != nil {
				errCh <- writeErr
				return
			}
		}
	}()

	observed, relayErr := awaitGrokRealtimeAudioObserved(errCh, &audioObserved)
	return time.Since(relayStarted), observed, relayErr
}

func grokVoiceInboundQuery(c *gin.Context) url.Values {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return nil
	}
	return c.Request.URL.Query()
}

func buildGrokVoiceHTTPRequestURL(baseURL, endpoint, method string, inbound url.Values) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if method != http.MethodGet || strings.Trim(strings.TrimSpace(endpoint), "/") != "custom-voices" {
		return u.String(), nil
	}
	query := u.Query()
	for _, key := range []string{"limit", "pagination_token"} {
		if value := inbound.Get(key); value != "" {
			query.Set(key, value)
		}
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func buildGrokRealtimeWebSocketURL(baseURL, model string, inbound url.Values) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(strings.TrimSpace(u.Scheme)) {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", fmt.Errorf("unsupported grok realtime URL scheme: %s", u.Scheme)
	}
	query := u.Query()
	query.Del("call_id")
	for _, key := range []string{"conversation_id", "reasoning.effort"} {
		if value := strings.TrimSpace(inbound.Get(key)); value != "" {
			query.Set(key, value)
		}
	}
	query.Set("model", firstNonEmpty(strings.TrimSpace(model), xai.DefaultRealtimeModel))
	u.RawQuery = query.Encode()
	return u.String(), nil
}

var ErrGrokCustomVoiceNotOwned = errors.New("grok custom voice is not owned by the authenticated user")
var ErrGrokRealtimeConversationNotOwned = errors.New("grok realtime conversation is not owned by the authenticated user")

const grokRealtimeConversationBindingTTL = 24 * time.Hour
const grokVoiceOwnershipMutationTimeout = 2 * time.Second

type grokVoiceHardAccountAffinityContextKey struct{}

func WithGrokVoiceHardAccountAffinity(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, grokVoiceHardAccountAffinityContextKey{}, true)
}

func grokVoiceHardAccountAffinityFromContext(ctx context.Context) bool {
	value, _ := ctx.Value(grokVoiceHardAccountAffinityContextKey{}).(bool)
	return value
}

type GrokVoiceRequestOwner struct {
	GroupID                 *int64
	UserID                  int64
	LibraryReservationToken string
}

type grokVoiceSessionAccountClaimer interface {
	ClaimSessionAccountIDWithTTL(ctx context.Context, groupID int64, sessionHash string, accountID int64, ttl time.Duration) (int64, error)
}

type grokVoiceLibraryReservationCache interface {
	ReserveGrokVoiceLibrary(ctx context.Context, groupID int64, libraryKey string, accountID int64, token string, ttl time.Duration) (bool, error)
	CommitGrokVoiceLibraryReservation(ctx context.Context, groupID int64, libraryKey, resourceKey string, accountID int64, token string) error
	ReleaseGrokVoiceLibraryReservation(ctx context.Context, groupID int64, libraryKey string, accountID int64, token string) error
}

var ErrGrokCustomVoiceCreateInProgress = errors.New("another custom voice creation is already in progress")

// Exceeds the maximum nine-minute Voice HTTP request window while bounding
// stale reservations when a process dies before ownership commit or release.
const grokVoiceLibraryReservationTTL = 10 * time.Minute

func resolveGrokVoiceRequestOwner(owners []GrokVoiceRequestOwner) (*GrokVoiceRequestOwner, error) {
	if len(owners) == 0 {
		return nil, nil
	}
	if len(owners) != 1 || owners[0].UserID <= 0 {
		return nil, fmt.Errorf("grok voice owner is invalid")
	}
	owner := owners[0]
	return &owner, nil
}

// GrokCustomVoiceLibrarySessionHash isolates the upstream voice library by
// group (the cache namespace) and downstream user. Rotating an API key does not
// orphan the user's voices.
func GrokCustomVoiceLibrarySessionHash(userID int64) string {
	if userID <= 0 {
		return ""
	}
	return "grok-custom-voice-library:" + DeriveSessionHashFromSeed(fmt.Sprintf("%d", userID))
}

func GrokCustomVoiceResourceSessionHash(userID int64, voiceID string) string {
	voiceID = strings.TrimSpace(voiceID)
	if userID <= 0 || !IsGrokCustomVoiceID(voiceID) {
		return ""
	}
	return "grok-custom-voice-resource:" + DeriveSessionHashFromSeed(fmt.Sprintf("%d:%s", userID, voiceID))
}

func GrokRealtimeConversationSessionHash(userID int64, conversationID string) string {
	conversationID = strings.TrimSpace(conversationID)
	if userID <= 0 || conversationID == "" {
		return ""
	}
	return "grok-realtime-conversation:" + DeriveSessionHashFromSeed(fmt.Sprintf("%d:%s", userID, conversationID))
}

// IsGrokCustomVoiceID follows xAI's documented eight-character lowercase
// alphanumeric identifier format. Built-in names are not treated as custom IDs.
func IsGrokCustomVoiceID(voiceID string) bool {
	voiceID = strings.TrimSpace(voiceID)
	if len(voiceID) != 8 {
		return false
	}
	for _, ch := range voiceID {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') {
			return false
		}
	}
	return true
}

func (s *OpenAIGatewayService) ResolveGrokCustomVoiceLibraryAccount(
	ctx context.Context,
	groupID *int64,
	userID int64,
) (int64, error) {
	if s == nil || s.cache == nil {
		return 0, fmt.Errorf("grok custom voice binding cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(GrokCustomVoiceLibrarySessionHash(userID))
	if cacheKey == "" {
		return 0, fmt.Errorf("grok custom voice binding is invalid")
	}
	return s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), cacheKey)
}

func (s *OpenAIGatewayService) BindGrokCustomVoiceLibraryAccount(
	ctx context.Context,
	groupID *int64,
	userID, accountID int64,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("grok custom voice binding cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(GrokCustomVoiceLibrarySessionHash(userID))
	if cacheKey == "" || accountID <= 0 {
		return fmt.Errorf("grok custom voice binding is invalid")
	}
	return s.claimGrokVoiceAccountBinding(ctx, groupID, cacheKey, accountID, "library")
}

func (s *OpenAIGatewayService) ReserveGrokCustomVoiceLibraryAccount(
	ctx context.Context,
	groupID *int64,
	userID, accountID int64,
) (string, error) {
	if s == nil || s.cache == nil {
		return "", fmt.Errorf("grok custom voice reservation cache is unavailable")
	}
	cache, ok := s.cache.(grokVoiceLibraryReservationCache)
	if !ok {
		return "", fmt.Errorf("grok custom voice reservation cache is unavailable")
	}
	libraryKey := s.openAISessionCacheKey(GrokCustomVoiceLibrarySessionHash(userID))
	if libraryKey == "" || accountID <= 0 {
		return "", fmt.Errorf("grok custom voice reservation is invalid")
	}
	token := uuid.NewString()
	reserved, err := cache.ReserveGrokVoiceLibrary(ctx, derefGroupID(groupID), libraryKey, accountID, token, grokVoiceLibraryReservationTTL)
	if err != nil {
		return "", err
	}
	if !reserved {
		return "", ErrGrokCustomVoiceCreateInProgress
	}
	return token, nil
}

func (s *OpenAIGatewayService) ReleaseGrokCustomVoiceLibraryReservation(
	ctx context.Context,
	groupID *int64,
	userID, accountID int64,
	token string,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("grok custom voice reservation cache is unavailable")
	}
	cache, ok := s.cache.(grokVoiceLibraryReservationCache)
	if !ok {
		return fmt.Errorf("grok custom voice reservation cache is unavailable")
	}
	libraryKey := s.openAISessionCacheKey(GrokCustomVoiceLibrarySessionHash(userID))
	if libraryKey == "" || accountID <= 0 || strings.TrimSpace(token) == "" {
		return fmt.Errorf("grok custom voice reservation is invalid")
	}
	// A failed upstream request commonly arrives together with request cancellation.
	// Reservation cleanup must outlive that cancellation, while remaining bounded.
	cleanupCtx, cancel := context.WithTimeout(context.Background(), grokVoiceOwnershipMutationTimeout)
	defer cancel()
	return cache.ReleaseGrokVoiceLibraryReservation(cleanupCtx, derefGroupID(groupID), libraryKey, accountID, token)
}

func (s *OpenAIGatewayService) commitGrokCustomVoiceLibraryReservation(
	ctx context.Context,
	owner GrokVoiceRequestOwner,
	accountID int64,
	voiceID string,
	token string,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("grok custom voice reservation cache is unavailable")
	}
	cache, ok := s.cache.(grokVoiceLibraryReservationCache)
	if !ok {
		return fmt.Errorf("grok custom voice reservation cache is unavailable")
	}
	libraryKey := s.openAISessionCacheKey(GrokCustomVoiceLibrarySessionHash(owner.UserID))
	resourceKey := s.openAISessionCacheKey(GrokCustomVoiceResourceSessionHash(owner.UserID, voiceID))
	if libraryKey == "" || resourceKey == "" || accountID <= 0 || strings.TrimSpace(token) == "" {
		return fmt.Errorf("grok custom voice reservation is invalid")
	}
	return cache.CommitGrokVoiceLibraryReservation(
		ctx, derefGroupID(owner.GroupID), libraryKey, resourceKey, accountID, token,
	)
}

func (s *OpenAIGatewayService) ResolveGrokCustomVoiceResourceAccount(
	ctx context.Context,
	groupID *int64,
	userID int64,
	voiceID string,
) (int64, error) {
	if s == nil || s.cache == nil {
		return 0, fmt.Errorf("grok custom voice ownership cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(GrokCustomVoiceResourceSessionHash(userID, voiceID))
	if cacheKey == "" {
		return 0, ErrGrokCustomVoiceNotOwned
	}
	accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), cacheKey)
	if errors.Is(err, ErrStickySessionNotFound) || accountID <= 0 {
		return 0, ErrGrokCustomVoiceNotOwned
	}
	return accountID, err
}

func (s *OpenAIGatewayService) BindGrokCustomVoiceResourceAccount(
	ctx context.Context,
	groupID *int64,
	userID int64,
	voiceID string,
	accountID int64,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("grok custom voice ownership cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(GrokCustomVoiceResourceSessionHash(userID, voiceID))
	if cacheKey == "" || accountID <= 0 {
		return fmt.Errorf("grok custom voice ownership binding is invalid")
	}
	return s.claimGrokVoiceAccountBinding(ctx, groupID, cacheKey, accountID, "resource")
}

func (s *OpenAIGatewayService) claimGrokVoiceAccountBinding(
	ctx context.Context,
	groupID *int64,
	cacheKey string,
	accountID int64,
	kind string,
) error {
	return s.claimGrokVoiceAccountBindingWithTTL(ctx, groupID, cacheKey, accountID, kind, 0)
}

func (s *OpenAIGatewayService) claimGrokVoiceAccountBindingWithTTL(
	ctx context.Context,
	groupID *int64,
	cacheKey string,
	accountID int64,
	kind string,
	ttl time.Duration,
) error {
	claimer, ok := s.cache.(grokVoiceSessionAccountClaimer)
	if !ok {
		return fmt.Errorf("grok custom voice ownership cache does not support atomic account claims")
	}
	boundAccountID, err := claimer.ClaimSessionAccountIDWithTTL(
		ctx, derefGroupID(groupID), cacheKey, accountID, ttl,
	)
	if err != nil {
		return err
	}
	if boundAccountID != accountID {
		return fmt.Errorf("grok custom voice %s is already bound to another account", kind)
	}
	return nil
}

func (s *OpenAIGatewayService) DeleteGrokCustomVoiceResourceAccount(
	ctx context.Context,
	groupID *int64,
	userID int64,
	voiceID string,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("grok custom voice ownership cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(GrokCustomVoiceResourceSessionHash(userID, voiceID))
	if cacheKey == "" {
		return fmt.Errorf("grok custom voice ownership binding is invalid")
	}
	return s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), cacheKey)
}

func (s *OpenAIGatewayService) ResolveGrokRealtimeConversationAccount(
	ctx context.Context,
	groupID *int64,
	userID int64,
	conversationID string,
) (int64, error) {
	if s == nil || s.cache == nil {
		return 0, fmt.Errorf("grok realtime conversation binding cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(GrokRealtimeConversationSessionHash(userID, conversationID))
	if cacheKey == "" {
		return 0, ErrGrokRealtimeConversationNotOwned
	}
	accountID, err := s.cache.GetSessionAccountID(ctx, derefGroupID(groupID), cacheKey)
	if errors.Is(err, ErrStickySessionNotFound) || accountID <= 0 {
		return 0, ErrGrokRealtimeConversationNotOwned
	}
	return accountID, err
}

func (s *OpenAIGatewayService) BindGrokRealtimeConversationAccount(
	ctx context.Context,
	groupID *int64,
	userID int64,
	conversationID string,
	accountID int64,
) error {
	if s == nil || s.cache == nil {
		return fmt.Errorf("grok realtime conversation binding cache is unavailable")
	}
	cacheKey := s.openAISessionCacheKey(GrokRealtimeConversationSessionHash(userID, conversationID))
	if cacheKey == "" || accountID <= 0 {
		return fmt.Errorf("grok realtime conversation binding is invalid")
	}
	return s.claimGrokVoiceAccountBindingWithTTL(
		ctx, groupID, cacheKey, accountID, "realtime conversation", grokRealtimeConversationBindingTTL,
	)
}

// ResolveGrokRealtimeResumeAccount validates a client-supplied conversation
// identifier and returns the account that originally created it.
func (s *OpenAIGatewayService) ResolveGrokRealtimeResumeAccount(
	ctx context.Context,
	groupID *int64,
	userID int64,
	conversationID string,
) (int64, string, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return 0, "", ErrGrokRealtimeConversationNotOwned
	}
	accountID, err := s.ResolveGrokRealtimeConversationAccount(ctx, groupID, userID, conversationID)
	if err != nil {
		return 0, "", err
	}
	return accountID, GrokRealtimeConversationSessionHash(userID, conversationID), nil
}

// ResolveGrokRealtimeRoutingAccount resolves explicit resume identifiers first.
// A brand-new session may fall back to the user's Custom Voice library account
// so a subsequent session.update can safely reference an owned custom voice.
func (s *OpenAIGatewayService) ResolveGrokRealtimeRoutingAccount(
	ctx context.Context,
	groupID *int64,
	userID int64,
	conversationID string,
) (accountID int64, sessionHash string, customVoiceLibrary bool, err error) {
	if strings.TrimSpace(conversationID) != "" {
		accountID, sessionHash, err = s.ResolveGrokRealtimeResumeAccount(
			ctx, groupID, userID, conversationID,
		)
		return accountID, sessionHash, false, err
	}
	accountID, err = s.ResolveGrokCustomVoiceLibraryAccount(ctx, groupID, userID)
	if errors.Is(err, ErrStickySessionNotFound) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return accountID, GrokCustomVoiceLibrarySessionHash(userID), true, nil
}

func grokCustomVoiceEndpointID(endpoint string) string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(endpoint), "/"), "/")
	if len(parts) < 2 || parts[0] != "custom-voices" || !IsGrokCustomVoiceID(parts[1]) {
		return ""
	}
	return parts[1]
}

func ExtractGrokCustomVoiceReferences(body []byte) []string {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return nil
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	var walk func(value any, key string)
	walk = func(value any, key string) {
		switch typed := value.(type) {
		case map[string]any:
			for childKey, child := range typed {
				walk(child, strings.ToLower(strings.TrimSpace(childKey)))
			}
		case []any:
			for _, child := range typed {
				walk(child, key)
			}
		case string:
			voiceID := strings.TrimSpace(typed)
			if (key == "voice_id" || key == "voice") && IsGrokCustomVoiceID(voiceID) {
				seen[voiceID] = struct{}{}
			}
		}
	}
	walk(payload, "")
	result := make([]string, 0, len(seen))
	for voiceID := range seen {
		result = append(result, voiceID)
	}
	return result
}

func ExtractGrokRealtimeConversationIDs(body []byte) []string {
	if len(body) == 0 || !gjson.ValidBytes(body) || gjson.GetBytes(body, "type").String() != "conversation.created" {
		return nil
	}
	conversationID := strings.TrimSpace(gjson.GetBytes(body, "conversation.id").String())
	if conversationID == "" {
		return nil
	}
	return []string{conversationID}
}

func (s *OpenAIGatewayService) authorizeGrokVoicePayload(
	ctx context.Context,
	owner GrokVoiceRequestOwner,
	accountID int64,
	baseEndpoint, endpoint string,
	body []byte,
) error {
	voiceIDs := ExtractGrokCustomVoiceReferences(body)
	if baseEndpoint == "custom-voices" {
		if voiceID := grokCustomVoiceEndpointID(endpoint); voiceID != "" {
			voiceIDs = append(voiceIDs, voiceID)
		}
	}
	for _, voiceID := range voiceIDs {
		boundAccountID, err := s.ResolveGrokCustomVoiceResourceAccount(ctx, owner.GroupID, owner.UserID, voiceID)
		if err != nil {
			return err
		}
		if boundAccountID != accountID {
			return ErrGrokCustomVoiceNotOwned
		}
	}
	return nil
}

func (s *OpenAIGatewayService) applyGrokVoiceOwnershipToResponse(
	ctx context.Context,
	owner GrokVoiceRequestOwner,
	accountID int64,
	baseEndpoint, endpoint, method string,
	requestBody, responseBody []byte,
) ([]byte, error) {
	if baseEndpoint == "tts" {
		// Custom Voice references were resolved and matched to accountID before
		// the request was sent. Re-claiming them after a paid TTS succeeds adds a
		// Redis failure point without strengthening authorization.
		return responseBody, nil
	}
	if baseEndpoint != "custom-voices" {
		return responseBody, nil
	}

	voiceID := grokCustomVoiceEndpointID(endpoint)
	if voiceID != "" {
		if method == http.MethodDelete {
			if err := s.DeleteGrokCustomVoiceResourceAccount(ctx, owner.GroupID, owner.UserID, voiceID); err != nil {
				return nil, err
			}
			return responseBody, nil
		}
		if err := s.BindGrokCustomVoiceResourceAccount(ctx, owner.GroupID, owner.UserID, voiceID, accountID); err != nil {
			return nil, err
		}
		return responseBody, nil
	}

	if method == http.MethodPost {
		createdVoiceID := strings.TrimSpace(gjson.GetBytes(responseBody, "voice_id").String())
		if !IsGrokCustomVoiceID(createdVoiceID) {
			return nil, fmt.Errorf("grok custom voice create response has no valid voice_id")
		}
		if token := strings.TrimSpace(owner.LibraryReservationToken); token != "" {
			if err := s.commitGrokCustomVoiceLibraryReservation(ctx, owner, accountID, createdVoiceID, token); err != nil {
				if !s.grokCustomVoiceCreateOwnershipConfirmed(owner, accountID, createdVoiceID) {
					return nil, fmt.Errorf("commit grok custom voice ownership: %w", err)
				}
			}
			return responseBody, nil
		}
		if err := s.BindGrokCustomVoiceLibraryAccount(ctx, owner.GroupID, owner.UserID, accountID); err != nil {
			if !s.grokCustomVoiceCreateOwnershipConfirmed(owner, accountID, createdVoiceID) {
				return nil, err
			}
			return responseBody, nil
		}
		if err := s.BindGrokCustomVoiceResourceAccount(ctx, owner.GroupID, owner.UserID, createdVoiceID, accountID); err != nil {
			if !s.grokCustomVoiceCreateOwnershipConfirmed(owner, accountID, createdVoiceID) {
				return nil, err
			}
		}
		return responseBody, nil
	}

	filtered, err := s.filterGrokCustomVoiceList(ctx, owner, accountID, responseBody)
	if err != nil {
		return nil, err
	}
	if err := s.BindGrokCustomVoiceLibraryAccount(ctx, owner.GroupID, owner.UserID, accountID); err != nil {
		return nil, err
	}
	return filtered, nil
}

func (s *OpenAIGatewayService) grokCustomVoiceCreateOwnershipConfirmed(
	owner GrokVoiceRequestOwner,
	accountID int64,
	voiceID string,
) bool {
	ctx, cancel := context.WithTimeout(context.Background(), grokVoiceOwnershipMutationTimeout)
	defer cancel()
	libraryAccountID, libraryErr := s.ResolveGrokCustomVoiceLibraryAccount(ctx, owner.GroupID, owner.UserID)
	if libraryErr != nil || libraryAccountID != accountID {
		return false
	}
	resourceAccountID, resourceErr := s.ResolveGrokCustomVoiceResourceAccount(ctx, owner.GroupID, owner.UserID, voiceID)
	return resourceErr == nil && resourceAccountID == accountID
}

func (s *OpenAIGatewayService) filterGrokCustomVoiceList(ctx context.Context, owner GrokVoiceRequestOwner, accountID int64, body []byte) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode grok custom voice list: %w", err)
	}
	var voices []json.RawMessage
	if err := json.Unmarshal(payload["voices"], &voices); err != nil {
		return nil, fmt.Errorf("decode grok custom voice entries: %w", err)
	}
	filtered := make([]json.RawMessage, 0, len(voices))
	for _, voice := range voices {
		voiceID := strings.TrimSpace(gjson.GetBytes(voice, "voice_id").String())
		if !IsGrokCustomVoiceID(voiceID) {
			continue
		}
		boundAccountID, err := s.ResolveGrokCustomVoiceResourceAccount(ctx, owner.GroupID, owner.UserID, voiceID)
		if errors.Is(err, ErrGrokCustomVoiceNotOwned) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if boundAccountID == accountID {
			filtered = append(filtered, voice)
		}
	}
	encodedVoices, err := json.Marshal(filtered)
	if err != nil {
		return nil, err
	}
	payload["voices"] = encodedVoices
	return json.Marshal(payload)
}

func grokVoiceOwnershipHTTPStatus(err error) int {
	if errors.Is(err, ErrGrokCustomVoiceNotOwned) {
		return http.StatusNotFound
	}
	return http.StatusServiceUnavailable
}

func writeGrokVoiceOwnershipError(c *gin.Context, status int, message string) {
	if c == nil || c.Writer == nil || c.Writer.Written() {
		return
	}
	errType := "api_error"
	if status == http.StatusNotFound {
		errType = "not_found_error"
	}
	MarkResponseCommitted(c)
	c.AbortWithStatusJSON(status, gin.H{"error": gin.H{
		"type":    errType,
		"message": message,
	}})
}

func awaitGrokRealtimeAudioObserved(errCh <-chan error, audioObserved *atomic.Bool) (bool, error) {
	err := <-errCh
	if audioObserved == nil {
		return false, err
	}
	return audioObserved.Load(), err
}

func grokRealtimeEventHasAudio(msg []byte) bool {
	if !gjson.ValidBytes(msg) {
		return false
	}
	eventType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(msg, "type").String()))
	if !strings.Contains(eventType, "audio") || strings.Contains(eventType, "transcript") {
		return false
	}
	for _, path := range []string{"audio", "delta", "data"} {
		value := gjson.GetBytes(msg, path)
		if value.Type == gjson.String && strings.TrimSpace(value.String()) != "" {
			return true
		}
	}
	return false
}

// estimateGrokVoiceAudioUsage derives billing units from the request/response.
// TTS: million characters of input text; STT: hours approximated from request body size
// when duration is unknown; custom-voices: no units (nil).
func estimateGrokVoiceAudioUsage(endpoint string, reqBody []byte, contentType string, respBody []byte, elapsed time.Duration) *AudioUsage {
	switch strings.TrimSpace(endpoint) {
	case "tts":
		// Prefer JSON "input" / "text" fields; fallback to raw body length.
		chars := 0
		if gjson.ValidBytes(reqBody) {
			for _, key := range []string{"input", "text", "prompt"} {
				if s := strings.TrimSpace(gjson.GetBytes(reqBody, key).String()); s != "" {
					chars = len([]rune(s))
					break
				}
			}
		}
		if chars <= 0 {
			chars = len(reqBody)
		}
		if chars <= 0 {
			return nil
		}
		return &AudioUsage{Mode: "tts", DurationOrUnits: float64(chars) / 1_000_000.0}
	case "stt":
		// Prefer response duration when present; do not trust client duration_seconds alone
		// (under-report would underbill). Floor against body-size heuristic and elapsed.
		secs := 0.0
		if gjson.ValidBytes(respBody) {
			for _, path := range []string{"duration", "duration_seconds", "audio_duration", "usage.seconds"} {
				if v := gjson.GetBytes(respBody, path); v.Exists() && v.Type == gjson.Number && v.Float() > 0 {
					secs = v.Float()
					break
				}
			}
		}
		// Multipart / body size heuristic: ~16KB/s for compressed speech (lower bound).
		sizeFloor := 0.0
		if len(reqBody) > 0 {
			sizeFloor = float64(len(reqBody)) / 16000.0
		}
		clientSecs := 0.0
		if gjson.ValidBytes(reqBody) {
			if v := gjson.GetBytes(reqBody, "duration_seconds"); v.Exists() && v.Type == gjson.Number {
				clientSecs = v.Float()
			}
		}
		if secs <= 0 {
			secs = elapsed.Seconds()
		}
		if secs <= 0 {
			secs = clientSecs
		}
		if secs <= 0 {
			secs = sizeFloor
		}
		// Cap untrusted client under-report: if client duration is much smaller than
		// size/elapsed floors, bill the larger of floors (anti underbill).
		if clientSecs > 0 && secs == clientSecs {
			floor := sizeFloor
			if elapsed.Seconds() > floor {
				floor = elapsed.Seconds()
			}
			if floor > 0 && clientSecs < floor*0.5 {
				secs = floor
			}
		}
		if secs <= 0 {
			return nil
		}
		return &AudioUsage{Mode: "stt", DurationOrUnits: secs / 3600.0}
	case "realtime":
		mins := elapsed.Minutes()
		if mins <= 0 {
			return nil
		}
		return &AudioUsage{Mode: "realtime", DurationOrUnits: mins}
	default:
		return nil
	}
}
