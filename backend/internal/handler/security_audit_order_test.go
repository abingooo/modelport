package handler

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type promptAuditOrderCase struct {
	file       string
	function   string
	auditToken string
}

func TestPromptAuditGatePrecedesAccountBillingAndUpstreamSideEffects(t *testing.T) {
	tests := []promptAuditOrderCase{
		{file: "gateway_handler.go", function: "Messages", auditToken: "checkSecurityAudit"},
		{file: "gateway_handler_chat_completions.go", function: "ChatCompletions", auditToken: "checkSecurityAudit"},
		{file: "gateway_handler_responses.go", function: "Responses", auditToken: "checkSecurityAudit"},
		{file: "gemini_v1beta_handler.go", function: "GeminiV1BetaModels", auditToken: "checkSecurityAudit"},
		{file: "openai_gateway_handler.go", function: "Responses", auditToken: "checkSecurityAudit"},
		{file: "openai_gateway_handler.go", function: "Messages", auditToken: "checkSecurityAudit"},
		{file: "openai_chat_completions.go", function: "ChatCompletions", auditToken: "checkSecurityAudit"},
		{file: "openai_images.go", function: "Images", auditToken: "checkSecurityAudit"},
		{file: "grok_media.go", function: "handleGrokMedia", auditToken: "checkSecurityAudit"},
		{file: "openai_embeddings.go", function: "Embeddings", auditToken: "checkSecurityAudit"},
		{file: "openai_alpha_search.go", function: "AlphaSearch", auditToken: "checkSecurityAudit"},
		{file: "image_task_handler.go", function: "Submit", auditToken: "checkSecurityAuditBeforeSubmit"},
		{file: "gateway_web_search.go", function: "WebSearch", auditToken: "checkSecurityAudit"},
		{file: "grok_audio.go", function: "GrokVoice", auditToken: "checkSecurityAudit"},
	}
	sideEffectTokens := []string{
		"CheckBillingEligibility(", "SelectAccount", ".Forward", "acquireResponsesUserSlot(",
		"AcquireUserSlot", "TryAcquireUserSlot", "acquireImageGenerationSlot(",
		"h.tasks.Create(", "h.service.Submit(",
	}
	for _, tt := range tests {
		t.Run(tt.file+"/"+tt.function, func(t *testing.T) {
			functionSource := stripGoComments(goFunctionSource(t, tt.file, tt.function))
			auditIndex := strings.Index(functionSource, tt.auditToken)
			require.NotEqual(t, -1, auditIndex, "missing Prompt Audit gate")
			foundSideEffect := false
			for _, sideEffect := range sideEffectTokens {
				index := strings.Index(functionSource, sideEffect)
				if index < 0 {
					continue
				}
				foundSideEffect = true
				require.Lessf(t, auditIndex, index, "%s must run before %s", tt.auditToken, sideEffect)
			}
			require.True(t, foundSideEffect, "coverage case must contain a downstream side effect")
		})
	}
}

func TestGrokSpecialGatewaysAcquireUserSlotBeforeAccountScheduling(t *testing.T) {
	tests := []struct {
		file          string
		function      string
		userSlotToken string
	}{
		{file: "grok_audio.go", function: "GrokRealtime", userSlotToken: "acquireResponsesUserSlot("},
		{file: "grok_audio.go", function: "GrokVoice", userSlotToken: "acquireResponsesUserSlotForDetachedUpstream("},
		{file: "gateway_web_search.go", function: "WebSearch", userSlotToken: "AcquireUserSlotWithWait("},
	}
	for _, tt := range tests {
		t.Run(tt.file+"/"+tt.function, func(t *testing.T) {
			functionSource := stripGoComments(goFunctionSource(t, tt.file, tt.function))
			userSlotIndex := strings.Index(functionSource, tt.userSlotToken)
			accountSelectionIndex := strings.Index(functionSource, "SelectAccount")
			require.NotEqual(t, -1, userSlotIndex, "missing user concurrency gate")
			require.NotEqual(t, -1, accountSelectionIndex, "missing account scheduling")
			require.Less(t, userSlotIndex, accountSelectionIndex, "user slot must be held before account scheduling")
			require.Contains(t, functionSource, "defer userRelease()", "user slot must be released when the request ends")
		})
	}
}

func TestGrokVoiceUsesLibraryBindingAsSchedulerSession(t *testing.T) {
	functionSource := stripGoComments(goFunctionSource(t, "grok_audio.go", "GrokVoice"))
	callPattern := regexp.MustCompile(`(?s)SelectAccountWithSchedulerForCapability\(\s*selectionCtx,\s*apiKey\.GroupID,\s*"",\s*voiceSessionHash,`)
	require.Regexp(t, callPattern, functionSource, "custom voice affinity must be passed in the scheduler sessionHash position")
	require.Contains(t, functionSource, "WithGrokVoiceHardAccountAffinity(selectionCtx)", "bound Voice resources must use hard scheduler affinity")
	require.Contains(t, functionSource, "selection.Account.ID != boundVoiceAccountID", "a missing bound account must not silently migrate Voice resources")
	require.Regexp(t, regexp.MustCompile(`acquireResponsesAccountSlotForDetachedUpstream\(c,\s*apiKey\.GroupID,\s*"",`), functionSource, "account-slot admission must not rewrite Voice ownership bindings")
}

func TestGrokVoiceBillsConsumedResultBeforeHandlingForwardError(t *testing.T) {
	functionSource := stripGoComments(goFunctionSource(t, "grok_audio.go", "GrokVoice"))
	resultIndex := strings.Index(functionSource, "if result != nil")
	recordIndex := strings.Index(functionSource, "recordGrokVoiceUsage(")
	errorIndex := strings.Index(functionSource, "if forwardErr == nil")
	failoverIndex := strings.Index(functionSource, "errors.As(forwardErr")
	require.NotEqual(t, -1, resultIndex)
	require.NotEqual(t, -1, recordIndex)
	require.NotEqual(t, -1, errorIndex)
	require.NotEqual(t, -1, failoverIndex)
	require.Less(t, resultIndex, recordIndex)
	require.Less(t, recordIndex, errorIndex)
	require.Less(t, recordIndex, failoverIndex, "a paid 2xx result must be recorded before any error branch")
	require.Contains(t, functionSource, "libraryReservationToken != \"\" && result == nil", "an ambiguous post-success commit must retain its reservation until TTL")
}

func stripGoComments(source string) string {
	source = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(source, "")
	return regexp.MustCompile(`(?m)//.*$`).ReplaceAllString(source, "")
}

func goFunctionSource(t *testing.T, filename, functionName string) string {
	t.Helper()
	raw, err := os.ReadFile(filename)
	require.NoError(t, err)
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, filename, raw, 0)
	require.NoError(t, err)
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != functionName || function.Body == nil {
			continue
		}
		start := files.Position(function.Pos()).Offset
		end := files.Position(function.End()).Offset
		require.Greater(t, end, start)
		return string(raw[start:end])
	}
	t.Fatalf("function %s not found in %s", functionName, filename)
	return ""
}
