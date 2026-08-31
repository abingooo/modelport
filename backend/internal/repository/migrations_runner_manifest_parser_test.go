package repository

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseLegacyModelPortMigrationManifest_ValidRowsAndBlankLines(t *testing.T) {
	const (
		rawA     = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		trimmedA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		rawB     = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
		trimmedB = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	)
	manifest := strings.Join([]string{
		legacyModelPortMigrationManifestHeader,
		"",
		"001_first.sql\t" + rawA + "\t" + trimmedA,
		"  ",
		"002_second.sql\t" + rawB + "\t" + trimmedB,
		"",
	}, "\n")

	got, err := parseLegacyModelPortMigrationManifest([]byte(manifest))
	require.NoError(t, err)
	require.Equal(t, map[string]legacyModelPortArchivedMigrationDigest{
		"001_first.sql":  {raw: rawA, trimmed: trimmedA},
		"002_second.sql": {raw: rawB, trimmed: trimmedB},
	}, got)
}

func TestParseLegacyModelPortMigrationManifest_RejectsMalformedInput(t *testing.T) {
	const validSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	validRow := "001_first.sql\t" + validSHA + "\t" + validSHA

	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{name: "empty", input: "", wantErr: "manifest is empty"},
		{name: "wrong header", input: "# filename\traw\trunner\n" + validRow, wantErr: "unexpected"},
		{name: "header only", input: legacyModelPortMigrationManifestHeader + "\n", wantErr: "no migration rows"},
		{name: "too few fields", input: legacyModelPortMigrationManifestHeader + "\n001_first.sql\t" + validSHA, wantErr: "has 2 fields"},
		{name: "too many fields", input: legacyModelPortMigrationManifestHeader + "\n001_first.sql\t" + validSHA + "\t" + validSHA + "\textra", wantErr: "has 4 fields"},
		{name: "path traversal", input: legacyModelPortMigrationManifestHeader + "\n../001_first.sql\t" + validSHA + "\t" + validSHA, wantErr: "invalid migration name"},
		{name: "backslash path", input: legacyModelPortMigrationManifestHeader + "\nfoo\\001_first.sql\t" + validSHA + "\t" + validSHA, wantErr: "invalid migration name"},
		{name: "trailing name whitespace", input: legacyModelPortMigrationManifestHeader + "\n001_first.sql \t" + validSHA + "\t" + validSHA, wantErr: "invalid migration name"},
		{name: "non sql name", input: legacyModelPortMigrationManifestHeader + "\n001_first.txt\t" + validSHA + "\t" + validSHA, wantErr: "invalid migration name"},
		{name: "uppercase raw sha", input: legacyModelPortMigrationManifestHeader + "\n001_first.sql\t" + strings.ToUpper(validSHA) + "\t" + validSHA, wantErr: "invalid raw checksum"},
		{name: "short runner sha", input: legacyModelPortMigrationManifestHeader + "\n001_first.sql\t" + validSHA + "\tdeadbeef", wantErr: "invalid runner checksum"},
		{name: "non hex sha", input: legacyModelPortMigrationManifestHeader + "\n001_first.sql\t" + strings.Repeat("g", 64) + "\t" + validSHA, wantErr: "invalid raw checksum"},
		{name: "duplicate row", input: legacyModelPortMigrationManifestHeader + "\n" + validRow + "\n" + validRow, wantErr: "duplicate"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseLegacyModelPortMigrationManifest([]byte(tt.input))
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateLegacyModelPortManifestSHA_RequiresLowercaseSHA256(t *testing.T) {
	valid := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	got, err := validateLegacyModelPortManifestSHA(valid, "raw", "001_first.sql")
	require.NoError(t, err)
	require.Equal(t, valid, got)

	for _, value := range []string{
		strings.ToUpper(valid),
		valid[:63],
		valid + "0",
		strings.Repeat("z", 64),
	} {
		_, err := validateLegacyModelPortManifestSHA(value, "raw", "001_first.sql")
		require.Error(t, err)
	}
}

func TestLegacyModelPortArchivedMigrationDigests_AuthenticatesEmbeddedArchive(t *testing.T) {
	digests, err := legacyModelPortArchivedMigrationDigests()
	require.NoError(t, err)
	require.Len(t, digests, 38)

	for _, name := range []string{
		legacyModelPortOpenAICompatibleProvidersMigration,
		legacyModelPortChannelMonitorProvidersMigration,
		"187_add_deepseek_platform.sql",
		"224_prompt_audit_instruction_patch.sql",
	} {
		digest, ok := digests[name]
		require.Truef(t, ok, "missing authenticated archive entry %s", name)
		require.Len(t, digest.raw, 64)
		require.Len(t, digest.trimmed, 64)
	}

	_, err = legacyModelPortArchivedMigrationChecksum("999_not_in_archive.sql")
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing from manifest")
}

func TestParseLegacyModelPortConstraintLiterals_ValidatesMembershipAndEscapes(t *testing.T) {
	tests := []struct {
		name    string
		def     string
		want    map[string]struct{}
		wantErr string
	}{
		{
			name: "in list",
			def:  "CHECK ((platform)::text IN ('openai', 'grok'))",
			want: stringSet("openai", "grok"),
		},
		{
			name: "any array",
			def:  "CHECK ((platform)::text = ANY (ARRAY['openai'::text, 'grok'::text]))",
			want: stringSet("openai", "grok"),
		},
		{
			name: "quoted target identifier",
			def:  "CHECK ((\"platform\")::text IN ('openai', 'grok'))",
			want: stringSet("openai", "grok"),
		},
		{
			name: "parenthesized cast target",
			def:  "CHECK (((platform)::text) = ANY (ARRAY['openai'::text, 'grok'::text]))",
			want: stringSet("openai", "grok"),
		},
		{
			name: "doubled quote",
			def:  "CHECK (platform IN ('provider''s-gateway'))",
			want: stringSet("provider's-gateway"),
		},
		{
			name:    "identifier boundary",
			def:     "CHECK ((platform_name)::text IN ('openai'))",
			wantErr: "does not reference column platform",
		},
		{
			name:    "quoted similar identifier",
			def:     "CHECK ((\"platform_name\")::text IN ('openai'))",
			wantErr: "does not reference column platform",
		},
		{
			name:    "wrong target",
			def:     "CHECK ((provider)::text IN ('openai'))",
			wantErr: "does not reference column platform",
		},
		{
			name:    "target outside membership",
			def:     "CHECK (platform = 1 AND other IN ('openai', 'grok'))",
			wantErr: "target column platform is not part of an IN/ANY membership",
		},
		{
			name:    "target null check outside membership",
			def:     "CHECK (other IN ('openai', 'grok') AND platform IS NOT NULL)",
			wantErr: "target column platform is not part of an IN/ANY membership",
		},
		{
			name:    "negated membership",
			def:     "CHECK (NOT platform IN ('openai', 'grok'))",
			wantErr: "unsupported identifier or operator",
		},
		{
			name:    "membership with or branch",
			def:     "CHECK (platform IN ('openai', 'grok') OR other IS NULL)",
			wantErr: "unsupported identifier or operator",
		},
		{
			name:    "membership with and branch",
			def:     "CHECK (platform IN ('openai', 'grok') AND other IS NULL)",
			wantErr: "unsupported identifier or operator",
		},
		{
			name:    "membership compared with false",
			def:     "CHECK ((platform IN ('openai', 'grok')) = FALSE)",
			wantErr: "unsupported identifier or operator",
		},
		{
			name:    "function argument outside membership",
			def:     "CHECK (coalesce(other, platform) IN ('openai', 'grok'))",
			wantErr: "target column platform is not part of an IN/ANY membership",
		},
		{
			name:    "not membership",
			def:     "CHECK (platform = 'openai')",
			wantErr: "not an IN/ANY membership",
		},
		{
			name:    "no literals",
			def:     "CHECK (platform = ANY (ARRAY[]::text[]))",
			wantErr: "no SQL string literals",
		},
		{
			name:    "duplicate literal",
			def:     "CHECK (platform IN ('openai', 'openai'))",
			wantErr: "duplicate literal",
		},
		{
			name:    "unterminated literal",
			def:     "CHECK (platform IN ('openai))",
			wantErr: "unterminated",
		},
		{
			name:    "comment cannot provide target",
			def:     "CHECK ((other)::text = ANY (ARRAY['openai', 'grok'])) /* platform */",
			wantErr: "does not reference column platform",
		},
		{
			name:    "string cannot provide target",
			def:     "CHECK ((other)::text = ANY (ARRAY['openai', 'grok'])) AND note = 'platform'",
			wantErr: "does not reference column platform",
		},
		{
			name:    "line comment cannot provide target",
			def:     "CHECK ((other)::text = ANY (ARRAY['openai', 'grok'])) -- platform\n",
			wantErr: "does not reference column platform",
		},
		{
			name:    "dollar quote cannot provide target",
			def:     "CHECK ((other)::text = ANY (ARRAY['openai', 'grok'])) AND note = $$platform$$",
			wantErr: "does not reference column platform",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLegacyModelPortConstraintLiterals(tt.def, "platform")
			if tt.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestScanLegacyModelPortSQLStringLiterals_IgnoresCommentsAndQuotedIdentifiers(t *testing.T) {
	expression := "-- 'ignored'\nCHECK (platform IN ('openai', 'grok''s')) /* 'also ignored' */ AND \"quoted\" = 'tail'"
	got, err := scanLegacyModelPortSQLStringLiterals(expression)
	require.NoError(t, err)
	require.Equal(t, []string{"openai", "grok's", "tail"}, got)
}

func TestScanLegacyModelPortSQLStringLiterals_SupportsEscapeAndRejectsUnterminatedSyntax(t *testing.T) {
	got, err := scanLegacyModelPortSQLStringLiterals(`CHECK (platform IN (E'provider\'s', 'plain\value'))`)
	require.NoError(t, err)
	require.Equal(t, []string{"provider's", `plain\value`}, got)

	for _, expression := range []string{
		"CHECK (platform IN ('openai))",
		"CHECK (platform IN ('openai')) /* missing close",
		"CHECK (platform IN ($tag$openai))",
	} {
		_, err := scanLegacyModelPortSQLStringLiterals(expression)
		require.Error(t, err, expression)
	}
}

func TestContainsSQLIdentifier_IgnoresNonCodeAndHonorsIdentifierBoundaries(t *testing.T) {
	tests := []struct {
		name  string
		expr  string
		ident string
		want  bool
	}{
		{name: "bare identifier", expr: "CHECK (platform IN ('openai'))", ident: "platform", want: true},
		{name: "quoted identifier", expr: `CHECK ("platform" IN ('openai'))`, ident: "platform", want: true},
		{name: "similar bare identifier", expr: "CHECK (platform_name IN ('openai'))", ident: "platform", want: false},
		{name: "similar quoted identifier", expr: `CHECK ("platform_name" IN ('openai'))`, ident: "platform", want: false},
		{name: "comment", expr: "CHECK (other IN ('openai')) /* platform */", ident: "platform", want: false},
		{name: "string", expr: "CHECK (other IN ('platform'))", ident: "platform", want: false},
		{name: "empty identifier", expr: `CHECK ("" IN ('openai'))`, ident: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, containsSQLIdentifier(tt.expr, tt.ident))
		})
	}
}

func TestValidateLegacyModelPortCheckConstraint_RequiresExactAllowedSet(t *testing.T) {
	allowed := []map[string]struct{}{stringSet("openai", "grok"), stringSet("openai", "grok", "kimi")}

	require.NoError(t, validateLegacyModelPortCheckConstraint(
		legacyModelPortConstraintInfo{
			typeName:      "c",
			validated:     true,
			def:           "CHECK (platform IN ('openai', 'grok'))",
			targetColumns: []string{"platform"},
		},
		"platform",
		allowed,
	))
	require.NoError(t, validateLegacyModelPortCheckConstraint(
		legacyModelPortConstraintInfo{
			typeName:      " C ",
			validated:     true,
			def:           "CHECK (platform IN ('openai', 'grok', 'kimi'))",
			targetColumns: []string{"platform"},
		},
		"platform",
		allowed,
	))

	for _, info := range []legacyModelPortConstraintInfo{
		{typeName: "p", validated: true, def: "CHECK (platform IN ('openai', 'grok'))", targetColumns: []string{"platform"}},
		{typeName: "c", validated: false, def: "CHECK (platform IN ('openai', 'grok'))", targetColumns: []string{"platform"}},
		{typeName: "c", validated: true, def: "CHECK (platform IN ('openai', 'grok'))", targetColumns: []string{"platform", "other"}},
		{typeName: "c", validated: true, def: "CHECK (platform IN ('openai', 'grok'))", targetColumns: []string{"other"}},
		{typeName: "c", validated: true, def: "CHECK (platform IN ('openai'))", targetColumns: []string{"platform"}},
		{typeName: "c", validated: true, def: "CHECK (platform IN ('openai', 'grok', 'other'))", targetColumns: []string{"platform"}},
	} {
		err := validateLegacyModelPortCheckConstraint(info, "platform", allowed)
		require.Error(t, err)
	}
}
