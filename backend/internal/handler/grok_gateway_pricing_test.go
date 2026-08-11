package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGrokGatewayCapabilitiesRejectMissingExplicitGroupPrices(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		group      *service.Group
		register   func(*gin.Engine, *OpenAIGatewayHandler, *GatewayHandler)
		capability string
		websocket  bool
	}{
		{
			name: "web search", method: http.MethodPost, path: "/web_search", body: `{"query":"latest news"}`,
			group: &service.Group{Platform: service.PlatformGrok}, capability: "Web Search",
			register: func(r *gin.Engine, _ *OpenAIGatewayHandler, h *GatewayHandler) {
				r.POST("/web_search", h.WebSearch)
			},
		},
		{
			name: "tts", method: http.MethodPost, path: "/tts", body: `{"input":"hello"}`,
			group: &service.Group{Platform: service.PlatformGrok}, capability: "Voice",
			register: func(r *gin.Engine, h *OpenAIGatewayHandler, _ *GatewayHandler) {
				r.POST("/tts", func(c *gin.Context) { h.GrokVoice(c, "tts") })
			},
		},
		{
			name: "stt", method: http.MethodPost, path: "/stt", body: "audio",
			group: &service.Group{Platform: service.PlatformGrok}, capability: "Voice",
			register: func(r *gin.Engine, h *OpenAIGatewayHandler, _ *GatewayHandler) {
				r.POST("/stt", func(c *gin.Context) { h.GrokVoice(c, "stt") })
			},
		},
		{
			name: "custom voice requires realtime too", method: http.MethodGet, path: "/custom-voices",
			group: func() *service.Group {
				zero := 0.0
				return &service.Group{Platform: service.PlatformGrok, AudioTTSPricePerMillionChars: &zero}
			}(), capability: "Voice",
			register: func(r *gin.Engine, h *OpenAIGatewayHandler, _ *GatewayHandler) {
				r.GET("/custom-voices", func(c *gin.Context) { h.GrokVoice(c, "custom-voices") })
			},
		},
		{
			name: "realtime", method: http.MethodGet, path: "/realtime",
			group: &service.Group{Platform: service.PlatformGrok}, capability: "Realtime", websocket: true,
			register: func(r *gin.Engine, h *OpenAIGatewayHandler, _ *GatewayHandler) {
				r.GET("/realtime", h.GrokRealtime)
			},
		},
		{
			name: "video create", method: http.MethodPost, path: "/videos", body: `{"model":"grok-imagine-video-1.5-preview","resolution":"720p","prompt":"waves"}`,
			group: &service.Group{Platform: service.PlatformGrok}, capability: "Video",
			register: func(r *gin.Engine, h *OpenAIGatewayHandler, _ *GatewayHandler) {
				r.POST("/videos", h.GrokVideoGeneration)
			},
		},
		{
			name: "video lookup", method: http.MethodGet, path: "/videos/request-1",
			group: &service.Group{Platform: service.PlatformGrok}, capability: "Video",
			register: func(r *gin.Engine, h *OpenAIGatewayHandler, _ *GatewayHandler) {
				r.GET("/videos/:request_id", h.GrokVideoStatus)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(func(c *gin.Context) {
				groupID := int64(7)
				c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{ID: 11, GroupID: &groupID, Group: tt.group})
				c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 13, Concurrency: 1})
				c.Next()
			})
			tt.register(router, &OpenAIGatewayHandler{}, &GatewayHandler{})

			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			if tt.websocket {
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "websocket")
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusNotFound, w.Code)
			require.Contains(t, w.Body.String(), `"type":"not_found_error"`)
			require.Contains(t, w.Body.String(), tt.capability+" API is not available for this group")
			require.NotContains(t, strings.ToLower(w.Body.String()), "price")
		})
	}
}

func TestGrokVideoPricingGateAllowsExplicitZeroForRequestedTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	zero := 0.0
	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(7)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID: 11, GroupID: &groupID,
			Group: &service.Group{Platform: service.PlatformGrok, VideoPrice720P: &zero},
		})
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 13, Concurrency: 1})
		c.Next()
	})
	h := &OpenAIGatewayHandler{}
	router.POST("/videos", h.GrokVideoGeneration)

	req := httptest.NewRequest(http.MethodPost, "/videos", strings.NewReader(`{"model":"grok-imagine-video","resolution":"720p","prompt":"waves"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code, "explicit zero must pass the price gate and reach dependency validation")
	require.NotContains(t, w.Body.String(), "not available for this group")
}

func TestGrokRealtimeRejectsCallIDBeforeDependenciesOrUpgrade(t *testing.T) {
	gin.SetMode(gin.TestMode)
	zero := 0.0
	router := gin.New()
	router.Use(func(c *gin.Context) {
		groupID := int64(7)
		c.Set(string(middleware2.ContextKeyAPIKey), &service.APIKey{
			ID:      11,
			GroupID: &groupID,
			Group: &service.Group{
				Platform:                 service.PlatformGrok,
				AudioRealtimePricePerMin: &zero,
			},
		})
		c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 13, Concurrency: 1})
		c.Next()
	})
	router.GET("/realtime", (&OpenAIGatewayHandler{}).GrokRealtime)

	req := httptest.NewRequest(http.MethodGet, "/realtime?call_id=call-untrusted", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), `"type":"invalid_request_error"`)
	require.Contains(t, w.Body.String(), "call_id resume is not supported")
}
