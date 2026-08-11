package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type grokVoicePostSuccessCache struct {
	GatewayCache
	bindings          map[string]int64
	fallbackAccountID int64
	setCalls          int
	commitWrites      bool
	commitErr         error
}

func (c *grokVoicePostSuccessCache) GetSessionAccountID(_ context.Context, _ int64, key string) (int64, error) {
	if accountID, ok := c.bindings[key]; ok {
		return accountID, nil
	}
	if c.fallbackAccountID > 0 {
		return c.fallbackAccountID, nil
	}
	return 0, ErrStickySessionNotFound
}

func (c *grokVoicePostSuccessCache) SetSessionAccountID(_ context.Context, _ int64, key string, accountID int64, _ time.Duration) error {
	c.setCalls++
	c.bindings[key] = accountID
	return nil
}

func (c *grokVoicePostSuccessCache) ReserveGrokVoiceLibrary(context.Context, int64, string, int64, string, time.Duration) (bool, error) {
	return true, nil
}

func (c *grokVoicePostSuccessCache) CommitGrokVoiceLibraryReservation(
	_ context.Context,
	_ int64,
	libraryKey string,
	resourceKey string,
	accountID int64,
	_ string,
) error {
	if c.commitWrites {
		c.bindings[libraryKey] = accountID
		c.bindings[resourceKey] = accountID
	}
	return c.commitErr
}

func (c *grokVoicePostSuccessCache) ReleaseGrokVoiceLibraryReservation(context.Context, int64, string, int64, string) error {
	return nil
}

type grokVoiceErrorReader struct {
	err error
}

func (r grokVoiceErrorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func newGrokVoicePostSuccessTestContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, nil)
	return c, recorder
}

func newGrokVoicePostSuccessTestAccount() *Account {
	return &Account{
		ID:       30,
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key": "test-secret",
		},
	}
}

func TestForwardGrokVoice_CustomTTSSuccessDoesNotRebindOwnership(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &grokVoicePostSuccessCache{
		bindings:          make(map[string]int64),
		fallbackAccountID: 30,
	}
	svc := &OpenAIGatewayService{
		cache: cache,
		httpUpstream: &httpUpstreamStub{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"audio/mpeg"}},
			Body:       io.NopCloser(strings.NewReader("audio")),
		}},
	}
	c, recorder := newGrokVoicePostSuccessTestContext(http.MethodPost, "/v1/tts")
	groupID := int64(7)

	result, err := svc.ForwardGrokVoice(
		context.Background(), c, newGrokVoicePostSuccessTestAccount(), "tts",
		[]byte(`{"input":"hello","voice_id":"abcd1234"}`), "application/json",
		GrokVoiceRequestOwner{GroupID: &groupID, UserID: 10},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, result.AudioUsage)
	require.Equal(t, "tts", result.AudioUsage.Mode)
	require.Zero(t, cache.setCalls, "a pre-authorized voice must not be claimed again after paid TTS")
	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestForwardGrokVoice_TwoHundredBodyFailureReturnsBillableNonRetryableResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	readErr := errors.New("upstream body interrupted")
	svc := &OpenAIGatewayService{
		httpUpstream: &httpUpstreamStub{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"audio/mpeg"}},
			Body:       io.NopCloser(grokVoiceErrorReader{err: readErr}),
		}},
	}
	c, recorder := newGrokVoicePostSuccessTestContext(http.MethodPost, "/v1/tts")

	result, err := svc.ForwardGrokVoice(
		context.Background(), c, newGrokVoicePostSuccessTestAccount(), "tts",
		[]byte(`{"input":"bill me"}`), "application/json",
	)

	require.ErrorIs(t, err, readErr)
	require.NotNil(t, result)
	require.NotNil(t, result.AudioUsage)
	require.Positive(t, result.AudioUsage.DurationOrUnits)
	require.Equal(t, http.StatusBadGateway, recorder.Code)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "a consumed 2xx operation must never fail over")
}

func TestForwardGrokVoice_CreateCommitFailureHidesVoiceID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &grokVoicePostSuccessCache{
		bindings:  make(map[string]int64),
		commitErr: errors.New("redis unavailable"),
	}
	svc := &OpenAIGatewayService{
		cache: cache,
		httpUpstream: &httpUpstreamStub{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"voice_id":"abcd1234"}`)),
		}},
	}
	c, recorder := newGrokVoicePostSuccessTestContext(http.MethodPost, "/v1/custom-voices")
	groupID := int64(7)

	result, err := svc.ForwardGrokVoice(
		context.Background(), c, newGrokVoicePostSuccessTestAccount(), "custom-voices",
		[]byte(`{"name":"voice"}`), "application/json",
		GrokVoiceRequestOwner{GroupID: &groupID, UserID: 10, LibraryReservationToken: "token"},
	)

	require.Error(t, err)
	require.NotNil(t, result, "the 2xx upstream operation was consumed")
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "abcd1234")
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
}

func TestForwardGrokVoice_CreateAmbiguousCommitReconcilesBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cache := &grokVoicePostSuccessCache{
		bindings:     make(map[string]int64),
		commitWrites: true,
		commitErr:    errors.New("redis response lost after commit"),
	}
	svc := &OpenAIGatewayService{
		cache: cache,
		httpUpstream: &httpUpstreamStub{resp: &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"voice_id":"abcd1234"}`)),
		}},
	}
	c, recorder := newGrokVoicePostSuccessTestContext(http.MethodPost, "/v1/custom-voices")
	groupID := int64(7)

	result, err := svc.ForwardGrokVoice(
		context.Background(), c, newGrokVoicePostSuccessTestAccount(), "custom-voices",
		[]byte(`{"name":"voice"}`), "application/json",
		GrokVoiceRequestOwner{GroupID: &groupID, UserID: 10, LibraryReservationToken: "token"},
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"voice_id":"abcd1234"}`, recorder.Body.String())
}
