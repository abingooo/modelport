package securityaudit

import (
	"context"
	"strings"
	"unicode/utf8"
)

const (
	InstructionClientAll               = "all"
	InstructionClientCodexVSCode       = "codex_vscode"
	InstructionClientCodexCLI          = "codex_cli"
	InstructionClientCodexDesktop      = "codex_desktop"
	InstructionClientOpenCode          = "opencode"
	InstructionClientModelPortInternal = "modelport_internal"
	InstructionClientOther             = "other"
	InstructionClientUnknown           = "unknown"
)

var instructionDetectedClientTypes = []string{
	InstructionClientCodexVSCode,
	InstructionClientCodexCLI,
	InstructionClientCodexDesktop,
	InstructionClientOpenCode,
	InstructionClientModelPortInternal,
	InstructionClientOther,
	InstructionClientUnknown,
}

var validInstructionClientTypeSet = map[string]struct{}{
	InstructionClientAll:               {},
	InstructionClientCodexVSCode:       {},
	InstructionClientCodexCLI:          {},
	InstructionClientCodexDesktop:      {},
	InstructionClientOpenCode:          {},
	InstructionClientModelPortInternal: {},
	InstructionClientOther:             {},
	InstructionClientUnknown:           {},
}

var validInstructionDetectedClientTypeSet = map[string]struct{}{
	InstructionClientCodexVSCode:       {},
	InstructionClientCodexCLI:          {},
	InstructionClientCodexDesktop:      {},
	InstructionClientOpenCode:          {},
	InstructionClientModelPortInternal: {},
	InstructionClientOther:             {},
	InstructionClientUnknown:           {},
}

type trustedInternalInstructionClientContextKey struct{}

func WithTrustedInternalInstructionClient(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, trustedInternalInstructionClientContextKey{}, true)
}

func IsTrustedInternalInstructionClient(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	trusted, _ := ctx.Value(trustedInternalInstructionClientContextKey{}).(bool)
	return trusted
}

func ClassifyInstructionClient(userAgent string, trustedInternal bool) string {
	if trustedInternal {
		return InstructionClientModelPortInternal
	}
	userAgent = strings.TrimSpace(userAgent)
	if !validInstructionUserAgent(userAgent) {
		return InstructionClientUnknown
	}
	normalized := strings.ToLower(userAgent)
	switch {
	case strings.HasPrefix(normalized, "codex_vscode/"), strings.HasPrefix(normalized, "codex_vscode_copilot/"):
		return InstructionClientCodexVSCode
	case strings.HasPrefix(normalized, "codex_cli_rs/"), strings.HasPrefix(normalized, "codex-tui/"):
		return InstructionClientCodexCLI
	case strings.HasPrefix(normalized, "codex desktop/"), strings.HasPrefix(normalized, "codex_chatgpt_desktop/"):
		return InstructionClientCodexDesktop
	case strings.HasPrefix(normalized, "opencode/"):
		return InstructionClientOpenCode
	default:
		return InstructionClientOther
	}
}

func validInstructionUserAgent(userAgent string) bool {
	if userAgent == "" || !utf8.ValidString(userAgent) {
		return false
	}
	for _, value := range userAgent {
		if value < 0x20 || value == 0x7f {
			return false
		}
	}
	return true
}

func instructionUserAgentSnapshot(userAgent string) string {
	userAgent = strings.TrimSpace(userAgent)
	if !validInstructionUserAgent(userAgent) {
		return ""
	}
	const maxBytes = 512
	if len(userAgent) <= maxBytes {
		return userAgent
	}
	userAgent = userAgent[:maxBytes]
	for !utf8.ValidString(userAgent) {
		userAgent = userAgent[:len(userAgent)-1]
	}
	return userAgent
}

func normalizeInstructionClientTypes(values []string) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if _, ok := validInstructionClientTypeSet[value]; !ok {
			return nil, errInvalidInstructionClientType
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil, errInvalidInstructionClientType
	}
	if _, all := seen[InstructionClientAll]; all {
		if len(result) != 1 {
			return nil, errInvalidInstructionClientType
		}
		return []string{InstructionClientAll}, nil
	}
	result = result[:0]
	for _, clientType := range instructionDetectedClientTypes {
		if _, ok := seen[clientType]; ok {
			result = append(result, clientType)
		}
	}
	return result, nil
}

func validInstructionDetectedClientType(value string) bool {
	_, ok := validInstructionDetectedClientTypeSet[strings.ToLower(strings.TrimSpace(value))]
	return ok
}
