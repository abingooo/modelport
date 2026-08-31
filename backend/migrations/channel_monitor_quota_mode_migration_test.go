package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorQuotaModeMigration(t *testing.T) {
	content, err := FS.ReadFile("226_channel_monitor_quota_mode.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")

	// provider CHECK 两张表扩到 8 平台，且带精确集合守卫（仿 176 grok 迁移）。
	require.Contains(t, sql, "channel_monitors_provider_check")
	require.Contains(t, sql, "channel_monitor_request_templates_provider_check")
	require.Contains(t, sql, "CHECK (provider IN ('openai', 'anthropic', 'gemini', 'grok', 'antigravity', 'kimi', 'zhipu', 'deepseek'))")
	// A sentinel substring (such as just "kimi") would accept a partial
	// allow-list and leave the database in a state the runtime cannot use.
	require.NotContains(t, sql, "position('kimi'")
	require.Contains(t, sql, "regexp_matches(monitor_constraint_def")
	require.Contains(t, sql, "regexp_matches(template_constraint_def")
	// The exact literal set is not enough: the named CHECK must depend on the
	// table's provider column, and only that column, before it is considered
	// already migrated.
	require.Contains(t, sql, "c.conkey IS NOT NULL")
	require.Contains(t, sql, "c.conkey[1]")
	require.Contains(t, sql, "a.attname = 'provider'")
	require.Contains(t, sql, "c.conrelid = 'public.channel_monitors'::regclass")
	require.Contains(t, sql, "c.conrelid = 'public.channel_monitor_request_templates'::regclass")
	// Both the upstream eight-provider set and the terminal legacy-compatible
	// superset must be represented explicitly; ordering follows the SQL
	// literal arrays rather than an implementation-dependent catalog order.
	require.Contains(t, sql, "'antigravity', 'anthropic', 'deepseek', 'gemini', 'grok', 'kimi', 'openai', 'zhipu'")
	require.Contains(t, sql, "'antigravity', 'anthropic', 'deepseek', 'doubao', 'gemini', 'glm', 'grok', 'kimi', 'mimo', 'minimax', 'openai', 'qwen', 'zhipu'")

	// check_mode 三态，默认 probe。
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS check_mode VARCHAR(32) NOT NULL DEFAULT 'probe'")
	require.Contains(t, sql, "CHECK (check_mode IN ('probe', 'quota', 'quota_probe'))")

	// account_id 关联账号，账号删除置空（监控保留，运行时报「账号未关联」）。
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS account_id BIGINT REFERENCES accounts(id) ON DELETE SET NULL")
	require.Contains(t, sql, "CREATE INDEX IF NOT EXISTS idx_channel_monitors_account_id ON channel_monitors(account_id)")

	// 历史表配额快照列。
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS quota JSONB")

	// 公开设置默认关闭。
	require.Contains(t, sql, "VALUES ('channel_monitor_show_quota', 'false')")
	require.Contains(t, sql, "ON CONFLICT (key) DO NOTHING")
}
