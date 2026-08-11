package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ip"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GrokRealtime exposes xAI's native Voice Realtime WebSocket.
// Only Grok-platform API keys may use this endpoint.
const grokRealtimeAccountSlotSessionHash = ""

func (h *OpenAIGatewayHandler) GrokRealtime(c *gin.Context) {
	if c == nil || c.Request == nil || !isOpenAIWSUpgradeRequest(c.Request) {
		h.errorResponse(c, http.StatusUpgradeRequired, "invalid_request_error", "WebSocket upgrade required (Upgrade: websocket)")
		return
	}
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGrok {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Realtime API is not supported for this platform")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if !service.GroupHasExplicitGrokAudioPrice(apiKey.Group, "realtime") {
		rejectUnpricedGrokGatewayCapability(c, "Realtime")
		return
	}
	// call_id resumes are only safe after a trusted xAI webhook has established
	// the downstream user/account owner. This gateway does not expose SIP call
	// registration yet, so reject the identifier before scheduling or upgrading.
	if strings.TrimSpace(c.Query("call_id")) != "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "call_id resume is not supported")
		return
	}
	if !h.ensureResponsesDependencies(c, nil) {
		return
	}
	requestedModel := strings.TrimSpace(c.Query("model"))
	if requestedModel == "" {
		requestedModel = xai.DefaultRealtimeModel
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	streamStarted := false
	reqLog := requestLogger(c, "handler.openai_gateway.grok_realtime")
	// No SSE keepalive may be written before coderws.Accept; any response byte
	// would make the WebSocket upgrade impossible.
	userRelease, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userRelease != nil {
		defer userRelease()
	}
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}
	realtimeSessionHash := ""
	boundRealtimeAccountID := int64(0)
	conversationID := strings.TrimSpace(c.Query("conversation_id"))
	var err error
	boundRealtimeAccountID, realtimeSessionHash, _, err = h.gatewayService.ResolveGrokRealtimeRoutingAccount(
		c.Request.Context(), apiKey.GroupID, subject.UserID, conversationID,
	)
	if err != nil {
		if errors.Is(err, service.ErrGrokRealtimeConversationNotOwned) {
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Realtime session is not available")
		} else {
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Realtime session binding unavailable")
		}
		return
	}

	selectionCtx := c.Request.Context()
	if boundRealtimeAccountID > 0 {
		selectionCtx = service.WithGrokVoiceHardAccountAffinity(selectionCtx)
	}
	selection, _, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
		selectionCtx,
		apiKey.GroupID,
		"",
		realtimeSessionHash,
		requestedModel,
		nil,
		service.OpenAIUpstreamTransportHTTPSSE,
		// Grok only advertises chat_completions + media capabilities on HEAD.
		service.OpenAIEndpointCapabilityChatCompletions,
		false,
		false,
		false,
		service.PlatformGrok,
	)
	if err != nil || selection == nil || selection.Account == nil {
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available Grok accounts")
		return
	}
	if boundRealtimeAccountID > 0 && selection.Account.ID != boundRealtimeAccountID {
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Bound Realtime account is unavailable")
		return
	}
	upstreamModel := strings.TrimSpace(selection.Account.GetMappedModel(requestedModel))
	if upstreamModel == "" {
		if selection.ReleaseFunc != nil {
			selection.ReleaseFunc()
		}
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available Grok accounts")
		return
	}

	// Ownership affinity was already enforced during scheduling. Passing its hash
	// into slot admission would rewrite it with the generic sticky-session TTL.
	release, slotStatus := h.acquireResponsesAccountSlot(c, apiKey.GroupID, grokRealtimeAccountSlotSessionHash, selection, false, &streamStarted, reqLog)
	if slotStatus != openAISlotAcquireOK {
		return
	}
	if release != nil {
		defer release()
	}

	token, _, err := h.gatewayService.GetRequestCredential(c.Request.Context(), c, selection.Account)
	if err != nil {
		h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Grok credential unavailable")
		return
	}

	conn, err := coderws.Accept(c.Writer, c.Request, &coderws.AcceptOptions{CompressionMode: coderws.CompressionContextTakeover})
	if err != nil {
		return
	}
	defer func() { _ = conn.CloseNow() }()

	relayDuration, proxyErr := h.gatewayService.ProxyGrokRealtime(
		c.Request.Context(), c, conn, selection.Account, token, upstreamModel,
		service.GrokVoiceRequestOwner{GroupID: apiKey.GroupID, UserID: subject.UserID},
	)
	unexpectedProxyError := proxyErr != nil && !isExpectedGrokRealtimeClose(proxyErr)
	if proxyErr != nil {
		reqLog.Info("grok_realtime.proxy_failed", zap.Error(proxyErr))
	}
	// Both normal and failed relays consumed upstream audio time and must be
	// billed before an unexpected close returns from the handler.
	if result := newGrokRealtimeUsageResult(requestedModel, upstreamModel, relayDuration); result != nil {
		h.recordGrokVoiceUsage(c, apiKey, selection.Account, subscription, "realtime", nil, result)
	}
	if unexpectedProxyError {
		_ = conn.Close(coderws.StatusInternalError, "upstream realtime websocket failed")
		return
	}
}

func newGrokRealtimeUsageResult(requestedModel, upstreamModel string, elapsed time.Duration) *service.OpenAIForwardResult {
	if elapsed <= 0 {
		return nil
	}
	return &service.OpenAIForwardResult{
		// One durable id per WS session so retries cannot collapse or double under client ids.
		RequestID:     service.StableGrokRealtimeBillingRequestID(""),
		Model:         requestedModel,
		UpstreamModel: upstreamModel,
		Duration:      elapsed,
		AudioUsage:    &service.AudioUsage{Mode: "realtime", DurationOrUnits: elapsed.Minutes()},
	}
}

func isExpectedGrokRealtimeClose(err error) bool {
	if err == nil {
		return true
	}
	switch coderws.CloseStatus(err) {
	case coderws.StatusNormalClosure, coderws.StatusGoingAway,
		coderws.StatusNoStatusRcvd, coderws.StatusAbnormalClosure:
		return true
	default:
		return false
	}
}

// GrokVoice handles xAI Voice HTTP endpoints. endpoint is "tts", "stt", or "custom-voices".
func (h *OpenAIGatewayHandler) GrokVoice(c *gin.Context, endpoint string) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey.Group == nil || apiKey.Group.Platform != service.PlatformGrok {
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "Voice API is not supported for this platform")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	baseEndpoint := strings.Split(strings.Trim(strings.TrimSpace(endpoint), "/"), "/")[0]
	priced := service.GroupHasExplicitGrokAudioPrice(apiKey.Group, baseEndpoint)
	if baseEndpoint == "custom-voices" {
		priced = service.GroupHasExplicitGrokCustomVoicePricing(apiKey.Group)
	}
	if !priced {
		rejectUnpricedGrokGatewayCapability(c, "Voice")
		return
	}
	if !h.ensureResponsesDependencies(c, nil) {
		return
	}

	body, err := readGrokVoiceGatewayBody(c)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if baseEndpoint == "tts" {
		reqLog := requestLogger(c, "handler.openai_gateway.grok_voice", zap.String("endpoint", endpoint))
		// TTS bodies use {"input":"..."} (and variants). Normalize to chat messages so
		// content moderation extractors see the spoken text.
		auditBody := body
		if input := extractGrokTTSInputText(body); input != "" {
			if b, err := json.Marshal(map[string]any{
				"messages": []map[string]any{{"role": "user", "content": input}},
			}); err == nil {
				auditBody = b
			}
		}
		if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIChat, xai.DefaultTTSModel, auditBody); decision != nil && !decision.AllowNextStage {
			h.openAISecurityAuditError(c, decision)
			return
		}
	}
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	streamStarted := false
	reqLog := requestLogger(c, "handler.openai_gateway.grok_voice", zap.String("endpoint", endpoint))
	userRelease, acquired := h.acquireResponsesUserSlotForDetachedUpstream(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userRelease != nil {
		defer userRelease()
	}
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}
	contentType := c.GetHeader("Content-Type")
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/json"
	}

	failed := map[int64]struct{}{}
	var last *service.UpstreamFailoverError
	selectionModel := xai.DefaultTTSModel
	if baseEndpoint == "stt" {
		selectionModel = xai.DefaultSTTModel
	}
	voiceSessionHash := ""
	boundVoiceAccountID := int64(0)
	libraryReservationToken := ""
	switch baseEndpoint {
	case "custom-voices":
		endpointParts := strings.Split(strings.Trim(strings.TrimSpace(endpoint), "/"), "/")
		if len(endpointParts) > 1 {
			boundVoiceID := endpointParts[1]
			voiceSessionHash = service.GrokCustomVoiceResourceSessionHash(subject.UserID, boundVoiceID)
			boundVoiceAccountID, err = h.gatewayService.ResolveGrokCustomVoiceResourceAccount(
				c.Request.Context(), apiKey.GroupID, subject.UserID, boundVoiceID,
			)
			if err != nil {
				if errors.Is(err, service.ErrGrokCustomVoiceNotOwned) {
					h.errorResponse(c, http.StatusNotFound, "not_found_error", "Custom Voice is not available")
				} else {
					h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Custom Voice ownership state unavailable")
				}
				return
			}
		} else {
			boundVoiceAccountID, err = h.gatewayService.ResolveGrokCustomVoiceLibraryAccount(
				c.Request.Context(), apiKey.GroupID, subject.UserID,
			)
			if errors.Is(err, service.ErrStickySessionNotFound) {
				boundVoiceAccountID = 0
				voiceSessionHash = ""
				if c.Request.Method == http.MethodGet {
					c.JSON(http.StatusOK, gin.H{"voices": []any{}, "pagination_token": nil})
					return
				}
			} else if err != nil {
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Custom Voice account binding unavailable")
				return
			} else {
				voiceSessionHash = service.GrokCustomVoiceLibrarySessionHash(subject.UserID)
			}
		}
	case "tts":
		for _, voiceID := range service.ExtractGrokCustomVoiceReferences(body) {
			accountID, resolveErr := h.gatewayService.ResolveGrokCustomVoiceResourceAccount(
				c.Request.Context(), apiKey.GroupID, subject.UserID, voiceID,
			)
			if resolveErr != nil || (boundVoiceAccountID > 0 && boundVoiceAccountID != accountID) {
				if resolveErr != nil && !errors.Is(resolveErr, service.ErrGrokCustomVoiceNotOwned) {
					h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Custom Voice ownership state unavailable")
				} else {
					h.errorResponse(c, http.StatusNotFound, "not_found_error", "Custom Voice is not available")
				}
				return
			}
			boundVoiceAccountID = accountID
			voiceSessionHash = service.GrokCustomVoiceResourceSessionHash(subject.UserID, voiceID)
		}
	}

	for attempts := 0; attempts < 4; attempts++ {
		selectionCtx := c.Request.Context()
		if boundVoiceAccountID > 0 {
			selectionCtx = service.WithGrokVoiceHardAccountAffinity(selectionCtx)
		}
		selection, _, selectErr := h.gatewayService.SelectAccountWithSchedulerForCapability(
			selectionCtx,
			apiKey.GroupID,
			"",
			voiceSessionHash,
			selectionModel,
			failed,
			service.OpenAIUpstreamTransportHTTPSSE,
			service.OpenAIEndpointCapabilityChatCompletions,
			false,
			false,
			false,
			service.PlatformGrok,
		)
		if selectErr != nil || selection == nil || selection.Account == nil {
			if last != nil {
				h.handleFailoverExhausted(c, last, false)
			} else {
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available Grok accounts")
			}
			return
		}
		if boundVoiceAccountID > 0 && selection.Account.ID != boundVoiceAccountID {
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
			h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Bound Custom Voice account is unavailable")
			return
		}
		account := selection.Account
		var started bool
		release, status := h.acquireResponsesAccountSlotForDetachedUpstream(c, apiKey.GroupID, "", selection, false, &started, reqLog)
		if status == openAISlotAcquireProfitVetoed {
			failed[account.ID] = struct{}{}
			continue
		}
		if status != openAISlotAcquireOK {
			// Failed already wrote error response (or transient reject).
			if status == openAISlotAcquireFailed && len(failed) == 0 {
				// Slot path wrote the response; stop.
				return
			}
			failed[account.ID] = struct{}{}
			continue
		}
		if baseEndpoint == "custom-voices" && c.Request.Method == http.MethodPost {
			libraryReservationToken, err = h.gatewayService.ReserveGrokCustomVoiceLibraryAccount(
				c.Request.Context(), apiKey.GroupID, subject.UserID, account.ID,
			)
			if err != nil {
				if release != nil {
					release()
				}
				if errors.Is(err, service.ErrGrokCustomVoiceCreateInProgress) {
					h.errorResponse(c, http.StatusConflict, "conflict_error", "Another Custom Voice creation is already in progress")
				} else {
					h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Custom Voice reservation unavailable")
				}
				return
			}
		}
		result, forwardErr := func() (*service.OpenAIForwardResult, error) {
			if release != nil {
				defer release()
			}
			return h.gatewayService.ForwardGrokVoice(
				c.Request.Context(), c, account, endpoint, body, contentType,
				service.GrokVoiceRequestOwner{
					GroupID: apiKey.GroupID, UserID: subject.UserID,
					LibraryReservationToken: libraryReservationToken,
				},
			)
		}()
		if result != nil {
			h.recordGrokVoiceUsage(c, apiKey, account, subscription, endpoint, body, result)
		}
		if forwardErr == nil {
			return
		}
		if libraryReservationToken != "" && result == nil {
			_ = h.gatewayService.ReleaseGrokCustomVoiceLibraryReservation(
				c.Request.Context(), apiKey.GroupID, subject.UserID, account.ID, libraryReservationToken,
			)
			libraryReservationToken = ""
		}
		var failoverErr *service.UpstreamFailoverError
		if errors.As(forwardErr, &failoverErr) && failoverErr.ShouldRetryNextAccount() {
			if baseEndpoint == "custom-voices" || boundVoiceAccountID > 0 {
				h.handleFailoverExhausted(c, failoverErr, false)
				return
			}
			failed[account.ID] = struct{}{}
			last = failoverErr
			continue
		}
		// Non-failover errors: handleGrokMediaErrorResponse / transport already wrote response.
		return
	}
	if last != nil {
		h.handleFailoverExhausted(c, last, false)
	}
}

// recordGrokVoiceUsage bills TTS/STT/realtime via group audio prices when AudioUsage is set.
func (h *OpenAIGatewayHandler) recordGrokVoiceUsage(
	c *gin.Context,
	apiKey *service.APIKey,
	account *service.Account,
	subscription *service.UserSubscription,
	endpoint string,
	body []byte,
	result *service.OpenAIForwardResult,
) {
	if h == nil || c == nil || apiKey == nil || account == nil || result == nil {
		return
	}
	if result.AudioUsage == nil {
		return
	}
	// Ensure forced durable request ids even if callers forget (realtime/tts/stt money path).
	if mode := strings.TrimSpace(result.AudioUsage.Mode); mode == "realtime" {
		result.RequestID = service.StableGrokRealtimeBillingRequestID(result.RequestID)
	} else {
		result.RequestID = service.StableGrokAudioBillingRequestID(result.RequestID)
	}
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	sessionID := service.ExtractClientSessionID(c)
	requestPayloadHash := service.HashUsageRequestPayload(body)
	if requestPayloadHash == "" {
		requestPayloadHash = service.HashUsageRequestPayload([]byte(endpoint))
	}
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	model := strings.TrimSpace(result.Model)
	if model == "" {
		model = endpoint
	}

	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result:             result,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: requestPayloadHash,
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
			SessionID:          sessionID,
			ChannelUsageFields: clientRequestedUsageFields(c, service.ChannelMappingResult{}, model, result.UpstreamModel),
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.grok_voice"),
				zap.Int64("user_id", apiKey.User.ID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("endpoint", endpoint),
				zap.Int64("account_id", account.ID),
			).Error("grok_voice.record_usage_failed", zap.Error(err))
		}
	})
}

func readGrokVoiceGatewayBody(c *gin.Context) ([]byte, error) {
	if c == nil || c.Request == nil {
		return nil, errors.New("request body is required")
	}
	if c.Request.Body == nil {
		if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodDelete {
			return nil, nil
		}
		return nil, errors.New("request body is required")
	}
	return io.ReadAll(c.Request.Body)
}

// extractGrokTTSInputText pulls the primary spoken text from a TTS JSON body.
func extractGrokTTSInputText(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	for _, key := range []string{"input", "text", "prompt"} {
		if v, ok := payload[key]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}
