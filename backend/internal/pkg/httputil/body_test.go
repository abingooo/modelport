package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

type countingRequestBody struct {
	reads int
}

func (body *countingRequestBody) Read([]byte) (int, error) {
	body.reads++
	return 0, io.EOF
}

func (body *countingRequestBody) Close() error { return nil }

const samplePayload = `{"model":"gpt-5.5","input":"hi","stream":false}`

func newRequestWithBody(t *testing.T, body []byte, encoding string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if encoding != "" {
		req.Header.Set("Content-Encoding", encoding)
	}
	req.ContentLength = int64(len(body))
	return req
}

func TestReadRequestBodyWithPrealloc_PassesThroughIdentity(t *testing.T) {
	req := newRequestWithBody(t, []byte(samplePayload), "")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_DecodesZstd(t *testing.T) {
	enc, _ := zstd.NewWriter(nil)
	compressed := enc.EncodeAll([]byte(samplePayload), nil)
	_ = enc.Close()

	req := newRequestWithBody(t, compressed, "zstd")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
	if req.Header.Get("Content-Encoding") != "" {
		t.Fatalf("Content-Encoding should be cleared after decoding")
	}
	if req.ContentLength != int64(len(samplePayload)) {
		t.Fatalf("ContentLength not updated: %d", req.ContentLength)
	}
}

func TestReadRequestBodyWithPrealloc_DecodesGzip(t *testing.T) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write([]byte(samplePayload)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}

	req := newRequestWithBody(t, buf.Bytes(), "gzip")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_DecodesDeflate(t *testing.T) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write([]byte(samplePayload)); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}

	req := newRequestWithBody(t, buf.Bytes(), "deflate")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_RejectsUnsupportedEncoding(t *testing.T) {
	req := newRequestWithBody(t, []byte(samplePayload), "br")
	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected error for unsupported encoding, got nil")
	}
	if !strings.Contains(err.Error(), "br") {
		t.Fatalf("error should mention encoding, got %v", err)
	}
}

func TestReadRequestBodyWithPrealloc_RejectsCorruptZstd(t *testing.T) {
	req := newRequestWithBody(t, []byte("not actually zstd"), "zstd")
	_, err := ReadRequestBodyWithPrealloc(req)
	if err == nil {
		t.Fatal("expected error for corrupt zstd body, got nil")
	}
}

func TestReadRequestBodyWithPrealloc_NilBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/v1/responses", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil body, got %q", got)
	}
}

func TestReadRequestBodyWithPrealloc_RespectsIdentityEncoding(t *testing.T) {
	req := newRequestWithBody(t, []byte(samplePayload), "identity")
	got, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != samplePayload {
		t.Fatalf("body mismatch: got %q", got)
	}
}

func TestReadRequestBodyBudgetRejectsBeforeReading(t *testing.T) {
	requestBody := &countingRequestBody{}
	request, err := http.NewRequest(http.MethodPost, "/v1/responses", requestBody)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	budget := NewRequestBodyMemoryBudget(64)

	_, lease, err := ReadRequestBodyWithPreallocLimitAndBudget(request, 64, 65, budget)
	if !errors.Is(err, ErrRequestBodyMemoryBudgetExceeded) {
		t.Fatalf("expected memory budget error, got %v", err)
	}
	if lease != nil {
		t.Fatal("failed reservation must not return a lease")
	}
	if requestBody.reads != 0 {
		t.Fatalf("body was read before reservation: reads=%d", requestBody.reads)
	}
}

func TestReadRequestBodyBudgetBlocksUntilLeaseReleased(t *testing.T) {
	const reservationBytes = int64(96)
	budget := NewRequestBodyMemoryBudget(reservationBytes)
	first := newRequestWithBody(t, []byte(samplePayload), "")
	_, firstLease, err := ReadRequestBodyWithPreallocLimitAndBudget(first, 1024, reservationBytes, budget)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if firstLease == nil {
		t.Fatal("first read did not return a lease")
	}

	result := make(chan error, 1)
	go func() {
		second := newRequestWithBody(t, []byte(samplePayload), "")
		_, lease, readErr := ReadRequestBodyWithPreallocLimitAndBudget(second, 1024, reservationBytes, budget)
		if lease != nil {
			lease.Release()
		}
		result <- readErr
	}()

	select {
	case readErr := <-result:
		t.Fatalf("second read bypassed held reservation: %v", readErr)
	case <-time.After(30 * time.Millisecond):
	}

	firstLease.Release()
	firstLease.Release()
	select {
	case readErr := <-result:
		if readErr != nil {
			t.Fatalf("second read after release: %v", readErr)
		}
	case <-time.After(time.Second):
		t.Fatal("second read did not resume after release")
	}
}

func TestReadRequestBodyBudgetHonorsContextCancellation(t *testing.T) {
	const reservationBytes = int64(96)
	budget := NewRequestBodyMemoryBudget(reservationBytes)
	lease, err := budget.Acquire(context.Background(), reservationBytes)
	if err != nil {
		t.Fatalf("initial reservation: %v", err)
	}
	defer lease.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "/v1/responses", strings.NewReader(samplePayload))
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	_, waitingLease, err := ReadRequestBodyWithPreallocLimitAndBudget(request, 1024, reservationBytes, budget)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if waitingLease != nil {
		t.Fatal("canceled reservation must not return a lease")
	}
}

func TestRequestBodyBudgetResizeKeepsExistingLeasesInOneBudget(t *testing.T) {
	budget := NewRequestBodyMemoryBudget(64)
	first, err := budget.Acquire(context.Background(), 64)
	if err != nil {
		t.Fatalf("initial reservation: %v", err)
	}
	defer first.Release()

	result := make(chan error, 1)
	go func() {
		lease, acquireErr := budget.Acquire(context.Background(), 64)
		if lease != nil {
			lease.Release()
		}
		result <- acquireErr
	}()
	select {
	case err := <-result:
		t.Fatalf("reservation bypassed original capacity: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	budget.SetCapacity(128)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("reservation after expansion: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("expanded budget did not wake the waiting reservation")
	}

	budget.SetCapacity(32)
	if _, err := budget.Acquire(context.Background(), 33); !errors.Is(err, ErrRequestBodyMemoryBudgetExceeded) {
		t.Fatalf("expected resized capacity error, got %v", err)
	}
}
