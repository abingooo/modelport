package migrations

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAICompatibleProvidersMigrationExtendsPlatformConstraints(t *testing.T) {
	sqlBytes, err := FS.ReadFile("188_add_openai_compatible_providers.sql")
	require.NoError(t, err)
	sql := string(sqlBytes)

	expectedPlatforms := []string{
		"anthropic", "openai", "gemini", "antigravity", "grok", "deepseek",
		"qwen", "glm", "kimi", "doubao", "siliconflow", "openrouter", "minimax", "mimo",
	}

	require.Equal(t, expectedPlatforms, migrationConstraintPlatforms(t, sql, "user_platform_quotas_platform_check"))
	require.Equal(t, expectedPlatforms, migrationConstraintPlatforms(t, sql, "composite_model_routes_target_platform_check"))
}

func migrationConstraintPlatforms(t *testing.T, migrationSQL, constraintName string) []string {
	t.Helper()

	constraintPattern := regexp.MustCompile(`(?s)ADD\s+CONSTRAINT\s+` + regexp.QuoteMeta(constraintName) + `\s+CHECK\s*\([^)]*\bIN\s*\(([^)]*)\)\s*\)`)
	constraintMatch := constraintPattern.FindStringSubmatch(migrationSQL)
	require.Len(t, constraintMatch, 2, "constraint %s must have one IN list", constraintName)

	valuePattern := regexp.MustCompile(`'([^']+)'`)
	valueMatches := valuePattern.FindAllStringSubmatch(constraintMatch[1], -1)
	platforms := make([]string, 0, len(valueMatches))
	for _, match := range valueMatches {
		platforms = append(platforms, match[1])
	}
	return platforms
}
