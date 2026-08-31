package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateRestoreDatabaseURLAcceptsExplicitIsolatedLoopbackTargets(t *testing.T) {
	t.Parallel()

	const suffix = "0123456789abcdef0123456789abcdef"
	for _, databaseURL := range []string{
		"postgres://modelport_restore_user_" + suffix + ":temporary@127.0.0.1:15432/modelport_restore_" + suffix + "?sslmode=disable",
		"postgres://modelport_restore_user_" + suffix + ":temporary@[::1]:35432/modelport_restore_" + suffix + "?sslmode=disable",
	} {
		databaseURL := databaseURL
		t.Run(databaseURL, func(t *testing.T) {
			t.Parallel()
			require.NoError(t, validateRestoreDatabaseURL(databaseURL))
		})
	}
}

func TestValidateRestoreDatabaseURLRejectsUnsafeTargets(t *testing.T) {
	t.Parallel()

	const (
		suffix      = "0123456789abcdef0123456789abcdef"
		otherSuffix = "abcdef0123456789abcdef0123456789"
		user        = "modelport_restore_user_" + suffix
		database    = "modelport_restore_" + suffix
	)
	tests := map[string]string{
		"empty":                     "",
		"wrong scheme":              "mysql://" + user + ":temporary@127.0.0.1:15432/" + database + "?sslmode=disable",
		"remote host":               "postgres://" + user + ":temporary@db.example.com:15432/" + database + "?sslmode=disable",
		"resolvable localhost name": "postgres://" + user + ":temporary@localhost:15432/" + database + "?sslmode=disable",
		"missing port":              "postgres://" + user + ":temporary@127.0.0.1/" + database + "?sslmode=disable",
		"missing user":              "postgres://127.0.0.1:15432/" + database + "?sslmode=disable",
		"missing password":          "postgres://" + user + "@127.0.0.1:15432/" + database + "?sslmode=disable",
		"ordinary user":             "postgres://restore:temporary@127.0.0.1:15432/" + database + "?sslmode=disable",
		"short user suffix":         "postgres://modelport_restore_user_drill:temporary@127.0.0.1:15432/" + database + "?sslmode=disable",
		"ordinary database":         "postgres://" + user + ":temporary@127.0.0.1:15432/sub2api?sslmode=disable",
		"short database suffix":     "postgres://" + user + ":temporary@127.0.0.1:15432/modelport_restore_drill?sslmode=disable",
		"uppercase database suffix": "postgres://" + user + ":temporary@127.0.0.1:15432/modelport_restore_0123456789abcdef0123456789abcdeF?sslmode=disable",
		"mismatched suffixes":       "postgres://" + user + ":temporary@127.0.0.1:15432/modelport_restore_" + otherSuffix + "?sslmode=disable",
		"missing ssl mode":          "postgres://" + user + ":temporary@127.0.0.1:15432/" + database,
		"unsafe ssl mode":           "postgres://" + user + ":temporary@127.0.0.1:15432/" + database + "?sslmode=require",
		"extra option":              "postgres://" + user + ":temporary@127.0.0.1:15432/" + database + "?sslmode=disable&host=db.example.com",
		"fragment":                  "postgres://" + user + ":temporary@127.0.0.1:15432/" + database + "?sslmode=disable#ignored",
		"nested path":               "postgres://" + user + ":temporary@127.0.0.1:15432/" + database + "/other?sslmode=disable",
	}

	for name, databaseURL := range tests {
		name, databaseURL := name, databaseURL
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Error(t, validateRestoreDatabaseURL(databaseURL))
		})
	}
}
