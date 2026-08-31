//go:build unit

package repository

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestPgDumperRestorePassesFailFastPSQLArguments(t *testing.T) {
	binDir := t.TempDir()
	argsFile := filepath.Join(t.TempDir(), "args")
	stdinFile := filepath.Join(t.TempDir(), "stdin")
	passwordFile := filepath.Join(t.TempDir(), "password")
	sslModeFile := filepath.Join(t.TempDir(), "sslmode")
	writeFakePSQL(t, binDir, `
printf '%s\n' "$@" > "$BACKUP_PG_DUMPER_ARGS_FILE"
cat > "$BACKUP_PG_DUMPER_STDIN_FILE"
printf '%s' "${PGPASSWORD-}" > "$BACKUP_PG_DUMPER_PASSWORD_FILE"
printf '%s' "${PGSSLMODE-}" > "$BACKUP_PG_DUMPER_SSLMODE_FILE"
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("BACKUP_PG_DUMPER_ARGS_FILE", argsFile)
	t.Setenv("BACKUP_PG_DUMPER_STDIN_FILE", stdinFile)
	t.Setenv("BACKUP_PG_DUMPER_PASSWORD_FILE", passwordFile)
	t.Setenv("BACKUP_PG_DUMPER_SSLMODE_FILE", sslModeFile)

	dumper := &PgDumper{cfg: &config.DatabaseConfig{
		Host:     "postgres.internal",
		Port:     5433,
		User:     "backup-user",
		Password: "test-password",
		DBName:   "modelport",
		SSLMode:  "require",
	}}

	err := dumper.Restore(context.Background(), strings.NewReader("SELECT 1;\n"))
	require.NoError(t, err)
	requireFileContent(t, argsFile, strings.Join([]string{
		"-h",
		"postgres.internal",
		"-p",
		"5433",
		"-U",
		"backup-user",
		"-d",
		"modelport",
		"--no-psqlrc",
		"--single-transaction",
		"--set=ON_ERROR_STOP=1",
	}, "\n")+"\n")
	requireFileContent(t, stdinFile, "SELECT 1;\n")
	requireFileContent(t, passwordFile, "test-password")
	requireFileContent(t, sslModeFile, "require")
}

func TestPgDumperRestoreReturnsErrorWhenPSQLStopsOnSQLError(t *testing.T) {
	binDir := t.TempDir()
	writeFakePSQL(t, binDir, `
on_error_stop=false
for arg in "$@"; do
	if [ "$arg" = "--set=ON_ERROR_STOP=1" ]; then
		on_error_stop=true
	fi
done
cat >/dev/null
if [ "$on_error_stop" = true ]; then
	printf '%s\n' 'ERROR: synthetic restore failure with api_key=sk-sensitive-row-value' >&2
	exit 3
fi
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	dumper := &PgDumper{cfg: &config.DatabaseConfig{
		Host:   "postgres.internal",
		Port:   5432,
		User:   "backup-user",
		DBName: "modelport",
	}}

	err := dumper.Restore(context.Background(), strings.NewReader("SELECT missing_column;\n"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "psql restore failed")
	require.Contains(t, err.Error(), "exit status 3")
	require.NotContains(t, err.Error(), "synthetic restore failure")
	require.NotContains(t, err.Error(), "sk-sensitive-row-value")
}

func writeFakePSQL(t *testing.T, binDir, body string) {
	t.Helper()
	path := filepath.Join(binDir, "psql")
	script := "#!/bin/sh\nset -eu\n" + body
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
}

func requireFileContent(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(content))
}
