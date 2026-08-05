package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
)

func extractMaxBytesError(err error) (*http.MaxBytesError, bool) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return maxErr, true
	}
	return nil, false
}

func formatBodyLimit(limit int64) string {
	const mb = 1024 * 1024
	if limit >= mb {
		return fmt.Sprintf("%dMB", limit/mb)
	}
	return fmt.Sprintf("%dB", limit)
}

func buildBodyTooLargeMessage(limit int64) string {
	return fmt.Sprintf("Request body too large, limit is %s", formatBodyLimit(limit))
}

func readLenientJSONRequestBodyWithPrealloc(req *http.Request, cfg *config.Config) ([]byte, error) {
	body, _, err := readLenientJSONRequestBodyWithAuditSource(req, cfg)
	return body, err
}

func readLenientJSONRequestBodyWithAuditSource(req *http.Request, cfg *config.Config) ([]byte, []byte, error) {
	decoded, err := pkghttputil.ReadRequestBodyWithPrealloc(req)
	if err != nil {
		return nil, nil, err
	}
	body, err := pkghttputil.NormalizeLenientJSONRequestBody(decoded, gatewayMaxBodySize(cfg))
	if err != nil {
		return nil, nil, err
	}
	return body, decoded, nil
}

func gatewayMaxBodySize(cfg *config.Config) int64 {
	if cfg == nil {
		return 0
	}
	return cfg.Gateway.MaxBodySize
}
