package securityaudit

import (
	"bytes"
	"encoding/json"
	"strings"
)

const instructionV2ResponsesEndpoint = "/v1/responses"

// InstructionV2RouteAllowed is the hard runtime boundary for Instruction Audit
// V2.  The protocol name is not sufficient because several non-generation
// gateways reuse the Responses protocol for compatibility.  Endpoints are
// compared against the concrete gateway aliases and WebSocket stages are
// additionally restricted to response.create frames.
func InstructionV2RouteAllowed(request Request) bool {
	if request.Protocol != instructionAuditProtocol || request.InstructionAuditExcluded {
		return false
	}
	if !instructionV2ResponsesGenerationEndpoint(request.Endpoint) {
		return false
	}

	switch strings.TrimSpace(request.Stage) {
	case "http":
		// The HTTP stage covers both JSON and SSE Responses generation.
		return true
	case "first_turn", "subsequent_turn":
		return instructionV2ResponseCreateFrame(instructionRequestBody(request))
	default:
		return false
	}
}

func instructionV2ResponsesGenerationEndpoint(endpoint string) bool {
	endpoint = strings.TrimSpace(endpoint)
	if index := strings.IndexAny(endpoint, "?#"); index >= 0 {
		endpoint = endpoint[:index]
	}
	endpoint = strings.TrimRight(endpoint, "/")
	switch endpoint {
	case instructionV2ResponsesEndpoint,
		"/openai/v1/responses",
		"/responses",
		"/backend-api/codex/responses":
		return true
	default:
		return false
	}
}

func instructionV2ResponseCreateFrame(body []byte) bool {
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return false
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return false
	}
	return envelope.Type == "response.create"
}
