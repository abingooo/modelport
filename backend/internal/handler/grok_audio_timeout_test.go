//go:build unit

package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokVoiceTimeoutUpstream struct {
	service.HTTPUpstream
	readStarted      chan struct{}
	bodyClosed       chan struct{}
	deadlineObserved chan time.Time
	inFlight         atomic.Int32
}

func (u *grokVoiceTimeoutUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	deadline, _ := req.Context().Deadline()
	u.deadlineObserved <- deadline
	u.inFlight.Add(1)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/octet-stream"}},
		Body: &grokVoiceTimeoutBody{
			ctx:         req.Context(),
			readStarted: u.readStarted,
			bodyClosed:  u.bodyClosed,
			inFlight:    &u.inFlight,
		},
	}, nil
}

type grokVoiceTimeoutBody struct {
	ctx         context.Context
	readStarted chan struct{}
	bodyClosed  chan struct{}
	inFlight    *atomic.Int32
	readOnce    sync.Once
	closeOnce   sync.Once
}

func (b *grokVoiceTimeoutBody) Read([]byte) (int, error) {
	b.readOnce.Do(func() { close(b.readStarted) })
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (b *grokVoiceTimeoutBody) Close() error {
	b.closeOnce.Do(func() {
		b.inFlight.Add(-1)
		close(b.bodyClosed)
	})
	return nil
}

func TestGrokVoiceHTTPTimeout_ReleasesDetachedRequestAndConcurrencySlots(t *testing.T) {
	gin.SetMode(gin.TestMode)
	groupID := int64(901)
	repo := &grokCredentialHandlerRepo{
		accounts: []service.Account{{
			ID: 801, Name: "voice", Platform: service.PlatformGrok, Type: service.AccountTypeAPIKey,
			Status: service.StatusActive, Schedulable: true, Concurrency: 1,
			Credentials: map[string]any{"api_key": "voice-key"},
			Extra:       map[string]any{service.GrokMediaEligibleExtraKey: true},
		}},
		missingOnGet: map[int64]bool{},
	}
	cfg := &config.Config{RunMode: config.RunModeSimple}
	cfg.Gateway.MaxAccountSwitches = 1
	cfg.Gateway.Grok.VoiceHTTPTimeoutSeconds = 1
	upstream := &grokVoiceTimeoutUpstream{
		readStarted:      make(chan struct{}),
		bodyClosed:       make(chan struct{}),
		deadlineObserved: make(chan time.Time, 1),
	}
	concurrencyCache := &concurrencyCacheMock{
		acquireUserSlotFn:    func(context.Context, int64, int, string) (bool, error) { return true, nil },
		acquireAccountSlotFn: func(context.Context, int64, int, string) (bool, error) { return true, nil },
	}
	concurrencyService := service.NewConcurrencyService(concurrencyCache)
	billingCache := service.NewBillingCacheService(nil, nil, nil, nil, nil, nil, cfg, nil)
	defer billingCache.Stop()
	gateway := service.NewOpenAIGatewayService(
		repo, nil, nil, nil, nil, nil, nil, cfg, nil, concurrencyService,
		service.NewBillingService(cfg, nil), nil, billingCache, upstream,
		&service.DeferredService{}, nil, nil, nil, nil, nil, nil, nil,
	)
	h := NewOpenAIGatewayHandler(
		gateway, concurrencyService, billingCache,
		&service.APIKeyService{}, nil, nil, nil, nil, cfg,
	)
	zeroPrice := 0.0
	apiKey := &service.APIKey{
		ID: 902, UserID: 903, GroupID: &groupID,
		User: &service.User{ID: 903, Status: service.StatusActive},
		Group: &service.Group{
			ID: groupID, Platform: service.PlatformGrok, Status: service.StatusActive,
			AudioTTSPricePerMillionChars: &zeroPrice,
		},
	}
	router := gin.New()
	router.Use(grokAuditTestMiddleware(apiKey))
	router.POST("/tts", func(c *gin.Context) { h.GrokVoice(c, "tts") })

	downstreamCtx, cancelDownstream := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/tts", strings.NewReader(""))
	request = request.WithContext(downstreamCtx)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		router.ServeHTTP(recorder, request)
	}()

	select {
	case <-upstream.readStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the upstream body read")
	}
	deadline := <-upstream.deadlineObserved
	require.False(t, deadline.IsZero(), "detached upstream request must have a deadline")
	require.WithinDuration(t, time.Now().Add(time.Second), deadline, 150*time.Millisecond)
	require.Equal(t, int32(1), upstream.inFlight.Load())

	cancelDownstream()
	select {
	case <-handlerDone:
		t.Fatal("downstream cancellation released the detached Voice request")
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case <-upstream.bodyClosed:
		t.Fatal("downstream cancellation closed the upstream response body")
	default:
	}
	require.Equal(t, int32(1), upstream.inFlight.Load())
	require.Zero(t, atomic.LoadInt32(&concurrencyCache.releaseUserCalled))
	require.Zero(t, atomic.LoadInt32(&concurrencyCache.releaseAccountCalled))

	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Voice HTTP deadline did not release the handler")
	}
	select {
	case <-upstream.bodyClosed:
	default:
		t.Fatal("Voice HTTP deadline did not close the upstream response body")
	}
	require.Equal(t, int32(0), upstream.inFlight.Load())
	require.Equal(t, int32(1), atomic.LoadInt32(&concurrencyCache.releaseUserCalled))
	require.Equal(t, int32(1), atomic.LoadInt32(&concurrencyCache.releaseAccountCalled))
	require.Equal(t, http.StatusBadGateway, recorder.Code, recorder.Body.String())
	require.Contains(t, recorder.Body.String(), "Upstream response could not be read")
}

var _ io.ReadCloser = (*grokVoiceTimeoutBody)(nil)
