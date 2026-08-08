package handler

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type requestBodyLimitInstructionEngine struct {
	budget *pkghttputil.RequestBodyMemoryBudget
	limit  int64
}

func (engine requestBodyLimitInstructionEngine) EvaluateInstruction(context.Context, securityaudit.Request) *securityaudit.InstructionDecision {
	return &securityaudit.InstructionDecision{Allow: true}
}

func (engine requestBodyLimitInstructionEngine) RequestBodyMemoryBudget() *pkghttputil.RequestBodyMemoryBudget {
	return engine.budget
}

func (engine requestBodyLimitInstructionEngine) RequestBodyReadLimit() int64 {
	return engine.limit
}

type requestBodyLimitRepeatingReader byte

func (reader requestBodyLimitRepeatingReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = byte(reader)
	}
	return len(buffer), nil
}

func sizedResponsesRequestBody(totalBytes int64, stream bool) io.Reader {
	prefix := fmt.Sprintf(`{"model":"gpt-test","stream":%t,"input":"`, stream)
	suffix := `"}`
	fillerBytes := totalBytes - int64(len(prefix)+len(suffix))
	return io.MultiReader(
		strings.NewReader(prefix),
		io.LimitReader(requestBodyLimitRepeatingReader('a'), fillerBytes),
		strings.NewReader(suffix),
	)
}

func TestLenientResponsesReaderPreservesDecodedStrictAuditSource(t *testing.T) {
	raw := []byte("{\"instructions\":\"line\nline\"}")
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write(raw)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(compressed.Bytes()))
	request.Header.Set("Content-Encoding", "gzip")
	normalized, auditSource, err := readLenientJSONRequestBodyWithAuditSource(request, nil)
	require.NoError(t, err)
	require.Equal(t, raw, auditSource)
	require.Contains(t, string(normalized), `\u000a`)
}

func TestLenientResponsesReaderRejectsDecodedBodyAboveGatewayLimit(t *testing.T) {
	raw := bytes.Repeat([]byte("a"), 65<<20)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write(raw)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(compressed.Bytes()))
	request.Header.Set("Content-Encoding", "gzip")
	_, _, err = readLenientJSONRequestBodyWithAuditSource(request, &config.Config{
		Gateway: config.GatewayConfig{MaxBodySize: 64 << 20},
	})
	maxErr, ok := extractMaxBytesError(err)
	require.True(t, ok)
	require.EqualValues(t, 64<<20, maxErr.Limit)
}

func TestLenientResponsesReaderPreservesDecodedBodyAboveInstructionLimit(t *testing.T) {
	prefix := []byte(`{"instructions":"unknown","metadata":"`)
	suffix := []byte(`"}`)
	raw := make([]byte, 65<<20)
	copied := copy(raw, prefix)
	for index := copied; index < len(raw)-len(suffix); index++ {
		raw[index] = 'a'
	}
	copy(raw[len(raw)-len(suffix):], suffix)
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write(raw)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(compressed.Bytes()))
	request.Header.Set("Content-Encoding", "gzip")
	_, auditSource, err := readLenientJSONRequestBodyWithAuditSource(request, &config.Config{
		Gateway: config.GatewayConfig{MaxBodySize: 128 << 20},
	})
	require.NoError(t, err)
	require.Len(t, auditSource, 65<<20)
}

func TestResponsesIngressBodySizeMatrix(t *testing.T) {
	const maxBodyBytes = int64(64 << 20)
	workingSetBytes, err := pkghttputil.RequestBodyWorkingSetBytes(maxBodyBytes, requestBodyWorkingSetBufferEstimate)
	require.NoError(t, err)
	budget := pkghttputil.NewRequestBodyMemoryBudget(workingSetBytes)

	transports := []struct {
		name   string
		stream bool
		accept string
	}{
		{name: "http", stream: false, accept: "application/json"},
		{name: "sse", stream: true, accept: "text/event-stream"},
	}
	for _, transport := range transports {
		for _, sizeMiB := range []int{1, 16, 32, 42, 64, 65} {
			t.Run(fmt.Sprintf("%s/%dMiB", transport.name, sizeMiB), func(t *testing.T) {
				targetBytes := int64(sizeMiB) << 20
				request := httptest.NewRequest(
					http.MethodPost,
					"/v1/responses",
					sizedResponsesRequestBody(targetBytes, transport.stream),
				)
				request.Header.Set("Accept", transport.accept)
				request.Header.Set("Content-Type", "application/json")

				normalized, auditSource, lease, readErr := readLenientJSONRequestBodyWithAuditSourceBudgetAndLimit(
					request, maxBodyBytes, budget,
				)
				if lease != nil {
					defer lease.Release()
				}

				if sizeMiB == 65 {
					var maxErr *http.MaxBytesError
					require.ErrorAs(t, readErr, &maxErr)
					require.Equal(t, maxBodyBytes, maxErr.Limit)
					require.Nil(t, normalized)
					require.Len(t, auditSource, int(maxBodyBytes+1))
					require.NotNil(t, lease)
					require.Equal(t, byte('{'), auditSource[0])
					return
				}

				require.NoError(t, readErr)
				require.NotNil(t, lease)
				require.Len(t, auditSource, int(targetBytes))
				require.Len(t, normalized, int(targetBytes))
				require.True(t, bytes.Equal(auditSource, normalized))
				require.Equal(t, byte('{'), normalized[0])
				require.Equal(t, byte('}'), normalized[len(normalized)-1])
			})
		}
	}
}

func TestLenientResponsesReaderBudgetCoversNormalizationLifetime(t *testing.T) {
	const maxBodyBytes = int64(1024)
	workingSetBytes, err := pkghttputil.RequestBodyWorkingSetBytes(maxBodyBytes, requestBodyWorkingSetBufferEstimate)
	require.NoError(t, err)
	budget := pkghttputil.NewRequestBodyMemoryBudget(workingSetBytes)
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxBodySize: maxBodyBytes}}

	first := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte("{\"instructions\":\"line\nline\"}")))
	normalized, auditSource, lease, err := readLenientJSONRequestBodyWithAuditSourceAndBudget(first, cfg, budget)
	require.NoError(t, err)
	require.NotNil(t, lease)
	require.Contains(t, string(normalized), `\u000a`)
	require.Contains(t, string(auditSource), "line\nline")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	second := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"instructions":"second"}`))).WithContext(ctx)
	_, _, secondLease, err := readLenientJSONRequestBodyWithAuditSourceAndBudget(second, cfg, budget)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, secondLease)

	lease.Release()
	third := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"instructions":"third"}`)))
	_, _, thirdLease, err := readLenientJSONRequestBodyWithAuditSourceAndBudget(third, cfg, budget)
	require.NoError(t, err)
	require.NotNil(t, thirdLease)
	thirdLease.Release()
}

func TestInstructionAuditDefaultBudgetUsesEffectiveAuditReadLimit(t *testing.T) {
	const (
		gatewayLimit = int64(256 << 20)
		auditLimit   = int64(64 << 20)
		budgetBytes  = int64(256 << 20)
	)
	budget := pkghttputil.NewRequestBodyMemoryBudget(budgetBytes)
	coordinator := securityaudit.NewCoordinatorWithInstruction(nil, nil, requestBodyLimitInstructionEngine{
		budget: budget,
		limit:  auditLimit,
	})
	effectiveLimit := instructionRequestBodyReadLimit(coordinator, gatewayLimit)
	require.Equal(t, auditLimit, effectiveLimit)

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"instructions":"trusted"}`)))
	_, _, readLease, err := readLenientJSONRequestBodyWithAuditSourceBudgetAndLimit(request, effectiveLimit, budget)
	require.NoError(t, err)
	require.NotNil(t, readLease)

	parserLease, err := budget.Acquire(context.Background(), auditLimit)
	require.NoError(t, err, "read double-buffer and parser buffer must fit the shared default budget")
	parserLease.Release()
	readLease.Release()
}

func TestLenientResponsesReaderReleasesBudgetAfterNormalizationError(t *testing.T) {
	const maxBodyBytes = int64(16)
	workingSetBytes, err := pkghttputil.RequestBodyWorkingSetBytes(maxBodyBytes, requestBodyWorkingSetBufferEstimate)
	require.NoError(t, err)
	budget := pkghttputil.NewRequestBodyMemoryBudget(workingSetBytes)
	cfg := &config.Config{Gateway: config.GatewayConfig{MaxBodySize: maxBodyBytes}}

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte("{\"x\":\"\x00\x00\x00\"}")))
	_, _, lease, err := readLenientJSONRequestBodyWithAuditSourceAndBudget(request, cfg, budget)
	var maxErr *http.MaxBytesError
	require.True(t, errors.As(err, &maxErr))
	require.Nil(t, lease)

	reservation, err := budget.Acquire(context.Background(), workingSetBytes)
	require.NoError(t, err)
	reservation.Release()
}

func TestRequestBodyLimitTooLarge(t *testing.T) {
	gin.SetMode(gin.TestMode)

	limit := int64(16)
	router := gin.New()
	router.Use(middleware.RequestBodyLimit(limit))
	router.POST("/test", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		if err != nil {
			if maxErr, ok := extractMaxBytesError(err); ok {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{
					"error": buildBodyTooLargeMessage(maxErr.Limit),
				})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "read_failed",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	payload := bytes.Repeat([]byte("a"), int(limit+1))
	req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(payload))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
	require.Contains(t, recorder.Body.String(), buildBodyTooLargeMessage(limit))
}
