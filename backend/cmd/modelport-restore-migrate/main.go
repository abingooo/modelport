// modelport-restore-migrate applies the embedded application migrations to an
// explicitly isolated PostgreSQL database used by a restore drill.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	_ "github.com/lib/pq"
)

const (
	restoreDatabaseURLEnv = "MODELPORT_RESTORE_DATABASE_URL"
	restoreDatabasePrefix = "modelport_restore_"
	restoreUserPrefix     = "modelport_restore_user_"
	restoreSuffixLength   = 32
	migrationTimeout      = 30 * time.Minute
)

func main() {
	databaseURL := strings.TrimSpace(os.Getenv(restoreDatabaseURLEnv))
	if err := validateRestoreDatabaseURL(databaseURL); err != nil {
		fmt.Fprintf(os.Stderr, "refusing restore migration target: %v\n", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), migrationTimeout)
	defer cancel()

	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "open isolated restore database failed")
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "connect to isolated restore database failed")
		os.Exit(1)
	}
	if err := repository.ApplyMigrations(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "apply migrations to isolated restore database: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("ModelPort migrations applied to isolated restore database")
}

func validateRestoreDatabaseURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("%s is required", restoreDatabaseURLEnv)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return errors.New("database URL is invalid")
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return errors.New("database URL must use postgres or postgresql")
	}
	if parsed.Opaque != "" || parsed.Fragment != "" {
		return errors.New("database URL must not be opaque or contain a fragment")
	}
	if parsed.User == nil || parsed.User.Username() == "" {
		return errors.New("database URL must include a temporary user")
	}
	userSuffix, ok := temporaryRestoreSuffix(parsed.User.Username(), restoreUserPrefix)
	if !ok {
		return fmt.Errorf("database user must be %q followed by exactly %d lowercase hexadecimal characters", restoreUserPrefix, restoreSuffixLength)
	}
	if password, ok := parsed.User.Password(); !ok || password == "" {
		return errors.New("database URL must include a temporary password")
	}

	host := strings.TrimSpace(parsed.Hostname())
	if !isLoopbackHost(host) {
		return errors.New("database host must be loopback")
	}
	portText := parsed.Port()
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("database URL must include an explicit valid port")
	}

	databaseName := strings.TrimPrefix(parsed.EscapedPath(), "/")
	unescapedName, err := url.PathUnescape(databaseName)
	if err != nil || databaseName == "" || strings.Contains(unescapedName, "/") {
		return errors.New("database URL must contain one database name")
	}
	databaseSuffix, ok := temporaryRestoreSuffix(unescapedName, restoreDatabasePrefix)
	if !ok {
		return fmt.Errorf("database name must be %q followed by exactly %d lowercase hexadecimal characters", restoreDatabasePrefix, restoreSuffixLength)
	}
	if databaseSuffix != userSuffix {
		return errors.New("temporary database and user suffixes must match")
	}

	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return errors.New("database URL query is invalid")
	}
	if len(query) != 1 || len(query["sslmode"]) != 1 || query.Get("sslmode") != "disable" {
		return errors.New("database URL must contain only sslmode=disable")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func temporaryRestoreSuffix(value, prefix string) (string, bool) {
	suffix, found := strings.CutPrefix(value, prefix)
	if !found || len(suffix) != restoreSuffixLength {
		return "", false
	}
	for _, character := range suffix {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return "", false
		}
	}
	return suffix, true
}
