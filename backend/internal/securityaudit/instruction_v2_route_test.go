package securityaudit

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInstructionV2RouteAllowlistRequiresResponsesGeneration(t *testing.T) {
	base := Request{
		Protocol:        instructionAuditProtocol,
		Endpoint:        "/v1/responses",
		Stage:           "http",
		InstructionBody: []byte(`{"instructions":"review me"}`),
	}

	for _, endpoint := range []string{
		"/v1/responses", "/v1/responses/", "/openai/v1/responses",
		"/responses", "/backend-api/codex/responses",
	} {
		t.Run("alias/"+endpoint, func(t *testing.T) {
			request := base
			request.Endpoint = endpoint
			require.True(t, InstructionV2RouteAllowed(request))
		})
	}

	for _, endpoint := range []string{
		"", "/v1/responses/compact", "/responses/compact",
		"/v1/responses/input_tokens", "/responses/input_tokens",
		"/v1/live", "/live", "/v1/realtime", "/realtime",
		"/v1/chat/completions", "/v1/messages", "/v1/responses/unknown",
	} {
		t.Run("excluded/"+endpoint, func(t *testing.T) {
			request := base
			request.Endpoint = endpoint
			require.False(t, InstructionV2RouteAllowed(request))
		})
	}

	for _, stage := range []string{"", "sse", "websocket", "relay", "first", "subsequent", "HTTP", "First_Turn", "SUBSEQUENT_TURN"} {
		t.Run("unknown-stage/"+stage, func(t *testing.T) {
			request := base
			request.Stage = stage
			require.False(t, InstructionV2RouteAllowed(request))
		})
	}

	for _, protocol := range []string{"openai_chat_completions", "anthropic_messages", "responses_websocket"} {
		t.Run("protocol/"+protocol, func(t *testing.T) {
			request := base
			request.Protocol = protocol
			require.False(t, InstructionV2RouteAllowed(request))
		})
	}
}

func TestInstructionV2RouteAllowlistRestrictsWebSocketFrames(t *testing.T) {
	for _, stage := range []string{"first_turn", "subsequent_turn"} {
		t.Run(stage, func(t *testing.T) {
			request := Request{
				Protocol:        instructionAuditProtocol,
				Endpoint:        "/v1/responses",
				Stage:           stage,
				InstructionBody: []byte(`{"type":"response.create","input":"hello"}`),
			}
			require.True(t, InstructionV2RouteAllowed(request))

			for _, body := range [][]byte{
				[]byte(`{"type":"session.update","session":{"instructions":"hello"}}`),
				[]byte(`{"type":"response.cancel","instructions":"hello"}`),
				[]byte(`{"instructions":"hello"}`),
				[]byte(`{"type":" response.create","input":"hello"}`),
				[]byte(`{"type":"response.create ","input":"hello"}`),
				[]byte(`{"type":"RESPONSE.CREATE","input":"hello"}`),
				[]byte(`not-json`),
			} {
				request.InstructionBody = body
				require.False(t, InstructionV2RouteAllowed(request), "body=%s", body)
			}
		})
	}

	for _, stage := range []string{" http ", " first_turn ", " subsequent_turn "} {
		request := Request{
			Protocol:        instructionAuditProtocol,
			Endpoint:        "/v1/responses",
			Stage:           stage,
			InstructionBody: []byte(`{"type":"response.create","input":"hello"}`),
		}
		if strings.TrimSpace(stage) == "http" {
			request.InstructionBody = []byte(`not-json`)
		}
		require.True(t, InstructionV2RouteAllowed(request), "stage=%q", stage)
	}

	request := Request{
		Protocol: instructionAuditProtocol,
		Endpoint: "/v1/responses",
		Stage:    "first_turn",
		Body:     []byte(`{"type":"response.create","input":"hello"}`),
	}
	require.True(t, InstructionV2RouteAllowed(request), "Body is the fallback when InstructionBody is empty")

	request.InstructionAuditExcluded = true
	require.False(t, InstructionV2RouteAllowed(request))
}
