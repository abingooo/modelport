package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
)

const (
	defaultGatewayRequestBodyBytes      int64 = 64 << 20
	requestBodyWorkingSetBufferEstimate int64 = 2
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
	body, auditSource, _, err := readLenientJSONRequestBodyWithAuditSourceAndBudget(req, cfg, nil)
	return body, auditSource, err
}

func readLenientJSONRequestBodyWithAuditSourceAndBudget(
	req *http.Request,
	cfg *config.Config,
	budget *pkghttputil.RequestBodyMemoryBudget,
) ([]byte, []byte, *pkghttputil.RequestBodyMemoryLease, error) {
	maxBodyBytes := gatewayMaxBodySize(cfg)
	return readLenientJSONRequestBodyWithAuditSourceBudgetAndLimit(req, maxBodyBytes, budget)
}

func readLenientJSONRequestBodyWithAuditSourceBudgetAndLimit(
	req *http.Request,
	maxBodyBytes int64,
	budget *pkghttputil.RequestBodyMemoryBudget,
) ([]byte, []byte, *pkghttputil.RequestBodyMemoryLease, error) {
	workingSetBytes, err := pkghttputil.RequestBodyWorkingSetBytes(maxBodyBytes, requestBodyWorkingSetBufferEstimate)
	if err != nil {
		return nil, nil, nil, err
	}
	decoded, lease, err := pkghttputil.ReadRequestBodyWithPreallocLimitAndBudget(req, maxBodyBytes, workingSetBytes, budget)
	if err != nil {
		if _, oversized := extractMaxBytesError(err); oversized && len(decoded) > 0 {
			return nil, decoded, lease, err
		}
		return nil, nil, nil, err
	}
	body, err := pkghttputil.NormalizeLenientJSONRequestBody(decoded, maxBodyBytes)
	if err != nil {
		lease.Release()
		return nil, nil, nil, err
	}
	return body, decoded, lease, nil
}

func gatewayMaxBodySize(cfg *config.Config) int64 {
	if cfg == nil || cfg.Gateway.MaxBodySize <= 0 {
		return defaultGatewayRequestBodyBytes
	}
	return cfg.Gateway.MaxBodySize
}
