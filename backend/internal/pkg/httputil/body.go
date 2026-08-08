package httputil

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
)

const (
	requestBodyReadInitCap    = 512
	requestBodyReadMaxInitCap = 1 << 20
	jsonUTF8BOMLen            = 3
	// maxDecompressedBodySize limits the decompressed request body to 64 MB
	// to prevent decompression bomb attacks.
	maxDecompressedBodySize = 64 << 20
)

// ErrRequestBodyMemoryBudgetExceeded reports a reservation larger than the
// configured process-local capacity.
var ErrRequestBodyMemoryBudgetExceeded = errors.New("request body memory reservation exceeds budget capacity")

// RequestBodyMemoryBudget bounds aggregate in-flight request-body working sets.
type RequestBodyMemoryBudget struct {
	mu       sync.Mutex
	capacity int64
	used     int64
	changed  chan struct{}
}

// RequestBodyMemoryLease keeps a reservation until all body consumers finish.
type RequestBodyMemoryLease struct {
	once   sync.Once
	budget *RequestBodyMemoryBudget
	weight int64
}

// NewRequestBodyMemoryBudget creates a process-local shared byte budget.
func NewRequestBodyMemoryBudget(capacity int64) *RequestBodyMemoryBudget {
	if capacity <= 0 {
		return nil
	}
	return &RequestBodyMemoryBudget{capacity: capacity, changed: make(chan struct{})}
}

// Capacity returns the configured budget size in bytes.
func (budget *RequestBodyMemoryBudget) Capacity() int64 {
	if budget == nil {
		return 0
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	return budget.capacity

}

// SetCapacity resizes the existing budget without separating requests that are
// already holding leases from requests admitted after a configuration reload.
func (budget *RequestBodyMemoryBudget) SetCapacity(capacity int64) {
	if budget == nil || capacity <= 0 {
		return
	}
	budget.mu.Lock()
	if budget.capacity != capacity {
		budget.capacity = capacity
		budget.notifyWaitersLocked()
	}
	budget.mu.Unlock()
}

// Acquire waits for a reservation or context cancellation.
func (budget *RequestBodyMemoryBudget) Acquire(ctx context.Context, weight int64) (*RequestBodyMemoryLease, error) {
	if budget == nil || weight <= 0 {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		budget.mu.Lock()
		capacity := budget.capacity
		if weight > capacity {
			budget.mu.Unlock()
			return nil, fmt.Errorf("%w: requested=%d capacity=%d", ErrRequestBodyMemoryBudgetExceeded, weight, capacity)
		}
		if budget.used <= capacity-weight {
			budget.used += weight
			budget.mu.Unlock()
			return &RequestBodyMemoryLease{budget: budget, weight: weight}, nil
		}
		changed := budget.changed
		budget.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

// Release returns the reservation and is safe to call more than once.
func (lease *RequestBodyMemoryLease) Release() {
	if lease == nil {
		return
	}
	lease.once.Do(func() {
		if lease.budget != nil && lease.weight > 0 {
			lease.budget.mu.Lock()
			lease.budget.used -= lease.weight
			if lease.budget.used < 0 {
				lease.budget.used = 0
			}
			lease.budget.notifyWaitersLocked()
			lease.budget.mu.Unlock()
		}
	})
}

func (budget *RequestBodyMemoryBudget) notifyWaitersLocked() {
	close(budget.changed)
	budget.changed = make(chan struct{})
}

// RequestBodyWorkingSetBytes computes a conservative multi-buffer reservation.
func RequestBodyWorkingSetBytes(maxBytes int64, simultaneousBuffers int64) (int64, error) {
	if maxBytes <= 0 || simultaneousBuffers <= 0 {
		return 0, nil
	}
	if maxBytes > (int64(^uint64(0)>>1) / simultaneousBuffers) {
		return 0, errors.New("request body working-set estimate overflows int64")
	}
	return maxBytes * simultaneousBuffers, nil
}

// ReadRequestBodyWithPrealloc reads request body with preallocated buffer based
// on content length, transparently decoding any Content-Encoding the upstream
// client used to compress the body (zstd, gzip, deflate).
func ReadRequestBodyWithPrealloc(req *http.Request) ([]byte, error) {
	return ReadRequestBodyWithPreallocLimit(req, maxDecompressedBodySize)
}

// ReadRequestBodyWithPreallocLimit reads and decompresses a request body while
// enforcing the same byte limit before and after decompression.
func ReadRequestBodyWithPreallocLimit(req *http.Request, maxBytes int64) ([]byte, error) {
	body, _, err := ReadRequestBodyWithPreallocLimitAndBudget(req, maxBytes, 0, nil)
	return body, err
}

// ReadRequestBodyWithPreallocLimitAndBudget reserves the caller-provided worst-case
// working set before allocating or reading request bytes. The caller must retain and
// release the returned lease after every consumer of the body has finished.
func ReadRequestBodyWithPreallocLimitAndBudget(
	req *http.Request,
	maxBytes int64,
	workingSetBytes int64,
	budget *RequestBodyMemoryBudget,
) ([]byte, *RequestBodyMemoryLease, error) {
	if req == nil || req.Body == nil {
		return nil, nil, nil
	}
	if maxBytes <= 0 {
		maxBytes = maxDecompressedBodySize
	}
	lease, err := budget.Acquire(req.Context(), workingSetBytes)
	if err != nil {
		return nil, nil, err
	}
	releaseOnError := true
	defer func() {
		if releaseOnError {
			lease.Release()
		}
	}()

	capHint := requestBodyReadInitCap
	if req.ContentLength > 0 {
		switch {
		case req.ContentLength < int64(requestBodyReadInitCap):
			capHint = requestBodyReadInitCap
		case req.ContentLength > int64(requestBodyReadMaxInitCap):
			capHint = requestBodyReadMaxInitCap
		default:
			capHint = int(req.ContentLength)
		}
	}

	if int64(capHint) > maxBytes {
		capHint = int(maxBytes)
	}
	buf := bytes.NewBuffer(make([]byte, 0, capHint))
	if _, err := io.Copy(buf, io.LimitReader(req.Body, maxBytes+1)); err != nil {
		return nil, nil, err
	}
	raw := buf.Bytes()
	if int64(len(raw)) > maxBytes {
		releaseOnError = false
		return raw, lease, &http.MaxBytesError{Limit: maxBytes}
	}

	enc := strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding")))
	if enc == "" || enc == "identity" {
		releaseOnError = false
		return raw, lease, nil
	}

	decoded, err := decompressRequestBody(enc, raw, maxBytes)
	if err != nil {
		if len(decoded) > 0 {
			releaseOnError = false
			return decoded, lease, fmt.Errorf("decode Content-Encoding %q: %w", enc, err)
		}
		return nil, nil, fmt.Errorf("decode Content-Encoding %q: %w", enc, err)
	}

	req.Header.Del("Content-Encoding")
	req.Header.Del("Content-Length")
	req.ContentLength = int64(len(decoded))

	releaseOnError = false
	return decoded, lease, nil
}

// ReadLenientJSONRequestBodyWithPrealloc reads a request body and normalizes
// JSON string control bytes before strict validation.
func ReadLenientJSONRequestBodyWithPrealloc(req *http.Request, maxNormalizedBytes int64) ([]byte, error) {
	body, err := ReadRequestBodyWithPrealloc(req)
	if err != nil {
		return nil, err
	}
	return NormalizeLenientJSONRequestBody(body, maxNormalizedBytes)
}

func decompressRequestBody(encoding string, raw []byte, maxBytes int64) ([]byte, error) {
	readDecoded := func(reader io.Reader) ([]byte, error) {
		decoded, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
		if err != nil {
			return nil, err
		}
		if int64(len(decoded)) > maxBytes {
			return decoded, &http.MaxBytesError{Limit: maxBytes}
		}
		return decoded, nil
	}
	switch encoding {
	case "zstd":
		dec, err := zstd.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer dec.Close()
		return readDecoded(dec)
	case "gzip", "x-gzip":
		gr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer func() { _ = gr.Close() }()
		return readDecoded(gr)
	case "deflate":
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer func() { _ = zr.Close() }()
		return readDecoded(zr)
	default:
		return nil, errors.New("unsupported Content-Encoding")
	}
}

// NormalizeLenientJSONRequestBody escapes raw control bytes that broken
// OpenAI-compatible clients sometimes place inside JSON strings.
func NormalizeLenientJSONRequestBody(body []byte, maxNormalizedBytes int64) ([]byte, error) {
	if maxNormalizedBytes <= 0 {
		maxNormalizedBytes = maxDecompressedBodySize
	}

	body = trimUTF8BOM(body)
	if len(body) == 0 {
		return body, nil
	}
	if int64(len(body)) > maxNormalizedBytes {
		return nil, &http.MaxBytesError{Limit: maxNormalizedBytes}
	}

	var out []byte
	inString := false
	escaped := false
	for i, b := range body {
		if inString && isJSONControlByte(b) {
			if out == nil {
				capHint := len(body) + 6
				if int64(capHint) > maxNormalizedBytes {
					capHint = int(maxNormalizedBytes)
				}
				out = make([]byte, 0, capHint)
				out = append(out, body[:i]...)
			}
			if int64(len(out)+6) > maxNormalizedBytes {
				return nil, &http.MaxBytesError{Limit: maxNormalizedBytes}
			}
			out = appendJSONUnicodeEscape(out, b)
			escaped = false
			continue
		}

		switch {
		case escaped:
			escaped = false
		case inString && b == '\\':
			escaped = true
		case b == '"':
			inString = !inString
		}

		if out != nil {
			if int64(len(out)+1) > maxNormalizedBytes {
				return nil, &http.MaxBytesError{Limit: maxNormalizedBytes}
			}
			out = append(out, b)
		}
	}
	if out != nil {
		return out, nil
	}
	return body, nil
}

func trimUTF8BOM(body []byte) []byte {
	if len(body) >= jsonUTF8BOMLen && body[0] == 0xef && body[1] == 0xbb && body[2] == 0xbf {
		return body[jsonUTF8BOMLen:]
	}
	return body
}

func isJSONControlByte(b byte) bool {
	return b < 0x20 || b == 0x7f
}

func appendJSONUnicodeEscape(dst []byte, b byte) []byte {
	const hex = "0123456789abcdef"
	return append(dst, '\\', 'u', '0', '0', hex[b>>4], hex[b&0x0f])
}
