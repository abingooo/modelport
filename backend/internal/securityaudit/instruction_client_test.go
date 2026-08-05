package securityaudit

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyInstructionClient(t *testing.T) {
	tests := []struct {
		name            string
		userAgent       string
		trustedInternal bool
		want            string
	}{
		{name: "trusted internal overrides user agent", userAgent: "codex_cli_rs/1.0.0", trustedInternal: true, want: InstructionClientModelPortInternal},
		{name: "vscode", userAgent: "codex_vscode/1.2.3", want: InstructionClientCodexVSCode},
		{name: "vscode copilot case insensitive", userAgent: "CODEX_VSCODE_COPILOT/1.2.3", want: InstructionClientCodexVSCode},
		{name: "cli", userAgent: "codex_cli_rs/0.145.0 (Windows)", want: InstructionClientCodexCLI},
		{name: "tui", userAgent: "codex-tui/0.145.0 (macOS)", want: InstructionClientCodexCLI},
		{name: "desktop", userAgent: "Codex Desktop/1.2.3", want: InstructionClientCodexDesktop},
		{name: "chatgpt desktop", userAgent: "codex_chatgpt_desktop/1.2.3", want: InstructionClientCodexDesktop},
		{name: "opencode", userAgent: "opencode/1.0.0", want: InstructionClientOpenCode},
		{name: "known token must be prefix", userAgent: "Mozilla/5.0 codex_cli_rs/0.145.0", want: InstructionClientOther},
		{name: "internal user agent is not trusted", userAgent: "modelport_internal/1.0", want: InstructionClientOther},
		{name: "other", userAgent: "curl/8.0", want: InstructionClientOther},
		{name: "missing", want: InstructionClientUnknown},
		{name: "invalid", userAgent: "client\x00/1.0", want: InstructionClientUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, ClassifyInstructionClient(test.userAgent, test.trustedInternal))
		})
	}
}

func TestNormalizeInstructionClientTypes(t *testing.T) {
	values, err := normalizeInstructionClientTypes([]string{" codex_cli ", "opencode", "codex_cli"})
	require.NoError(t, err)
	require.Equal(t, []string{InstructionClientCodexCLI, InstructionClientOpenCode}, values)

	_, err = normalizeInstructionClientTypes([]string{InstructionClientAll, InstructionClientCodexCLI})
	require.ErrorIs(t, err, errInvalidInstructionClientType)
	_, err = normalizeInstructionClientTypes([]string{"unsupported"})
	require.ErrorIs(t, err, errInvalidInstructionClientType)
	values, err = normalizeInstructionClientTypes(nil)
	require.NoError(t, err)
	require.Nil(t, values)
}

func TestInstructionUserAgentSnapshotIsBoundedUTF8(t *testing.T) {
	value := instructionUserAgentSnapshot(strings.Repeat("界", 300))
	require.LessOrEqual(t, len(value), 512)
	require.NotEmpty(t, value)
}

func TestTrustedInternalInstructionClientContext(t *testing.T) {
	require.False(t, IsTrustedInternalInstructionClient(context.Background()))
	require.True(t, IsTrustedInternalInstructionClient(WithTrustedInternalInstructionClient(context.Background())))
}
