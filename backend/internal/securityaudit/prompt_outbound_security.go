package securityaudit

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const maxGuardResponseBytes int64 = 256 * 1024

func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", infraerrors.BadRequest("prompt_audit_invalid_base_url", "审计节点地址无效")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", infraerrors.BadRequest("prompt_audit_invalid_base_url_scheme", "审计节点仅支持 HTTP(S)")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", infraerrors.BadRequest("prompt_audit_unsafe_base_url", "审计节点地址不能包含凭据、查询参数或片段")
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return "", infraerrors.BadRequest("prompt_audit_invalid_base_url", "审计节点地址无效")
	}
	path := strings.TrimRight(parsed.EscapedPath(), "/")
	if strings.EqualFold(path, "/v1") {
		path = ""
	}
	parsed.Path = path
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func ChatCompletionsURL(base string) (string, error) {
	return openAIEndpointURL(base, "/v1/chat/completions")
}

func ModelsURL(base string) (string, error) {
	return openAIEndpointURL(base, "/v1/models")
}

func openAIEndpointURL(base, endpoint string) (string, error) {
	normalized, err := NormalizeBaseURL(base)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", infraerrors.BadRequest("prompt_audit_invalid_base_url", "审计节点地址无效")
	}
	endpoint = "/" + strings.TrimLeft(strings.TrimSpace(endpoint), "/")
	relative := strings.TrimPrefix(endpoint, "/v1")
	path := strings.TrimRight(parsed.Path, "/")
	for _, resource := range []string{"/chat/completions", "/models"} {
		if strings.HasSuffix(path, resource) {
			prefix := strings.TrimSuffix(path, resource)
			if hasOpenAIAPIVersionSuffix(prefix) {
				path = prefix + relative
				parsed.Path = path
				parsed.RawPath = ""
				return parsed.String(), nil
			}
		}
	}
	if hasOpenAIAPIVersionSuffix(path) {
		path += relative
	} else {
		path += endpoint
	}
	parsed.Path = path
	parsed.RawPath = ""
	return parsed.String(), nil
}

func hasOpenAIAPIVersionSuffix(path string) bool {
	path = strings.TrimRight(strings.TrimSpace(path), "/")
	if path == "" {
		return false
	}
	segment := path[strings.LastIndex(path, "/")+1:]
	segment = strings.ToLower(strings.TrimSpace(segment))
	if len(segment) < 2 || segment[0] != 'v' || segment[1] < '0' || segment[1] > '9' {
		return false
	}
	i := 2
	for i < len(segment) && segment[i] >= '0' && segment[i] <= '9' {
		i++
	}
	if i == len(segment) {
		return true
	}
	if segment[i] == '.' {
		i++
		if i == len(segment) || segment[i] < '0' || segment[i] > '9' {
			return false
		}
		for i < len(segment) && segment[i] >= '0' && segment[i] <= '9' {
			i++
		}
		return i == len(segment)
	}
	suffix := segment[i:]
	return strings.HasPrefix(suffix, "alpha") ||
		strings.HasPrefix(suffix, "beta") ||
		strings.HasPrefix(suffix, "preview")
}

func NewSecureHTTPClient(endpoint ActiveEndpoint) (*http.Client, error) {
	_, err := NormalizeBaseURL(endpoint.BaseURL)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		// Do not inherit HTTP(S)_PROXY. A proxy would move the actual destination
		// dial outside secureDialContext and bypass this module's DNS/IP validation.
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: time.Duration(endpoint.TimeoutMS) * time.Millisecond,
		ExpectContinueTimeout: time.Second,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
	}
	// Endpoint ownership and destination trust are administrator concerns.
	// Use the standard dialer so configured private, loopback, reserved, and
	// DNS-resolved addresses are all reachable from the service environment.
	transport.DialContext = dialer.DialContext
	timeout := time.Duration(endpoint.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = DefaultTimeoutMS * time.Millisecond
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}, nil
}
