package setup

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestDecideAdminBootstrap(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		totalUsers int64
		adminUsers int64
		should     bool
		reason     string
	}{
		{
			name:       "empty database should create admin",
			totalUsers: 0,
			adminUsers: 0,
			should:     true,
			reason:     adminBootstrapReasonEmptyDatabase,
		},
		{
			name:       "admin exists should skip",
			totalUsers: 10,
			adminUsers: 1,
			should:     false,
			reason:     adminBootstrapReasonAdminExists,
		},
		{
			name:       "users exist without admin should skip",
			totalUsers: 5,
			adminUsers: 0,
			should:     false,
			reason:     adminBootstrapReasonUsersExistWithoutAdmin,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := decideAdminBootstrap(tc.totalUsers, tc.adminUsers)
			if got.shouldCreate != tc.should {
				t.Fatalf("shouldCreate=%v, want %v", got.shouldCreate, tc.should)
			}
			if got.reason != tc.reason {
				t.Fatalf("reason=%q, want %q", got.reason, tc.reason)
			}
		})
	}
}

func TestSetupDefaultAdminConcurrency(t *testing.T) {
	t.Run("simple mode admin uses higher concurrency", func(t *testing.T) {
		t.Setenv("RUN_MODE", "simple")
		if got := setupDefaultAdminConcurrency(); got != simpleModeAdminConcurrency {
			t.Fatalf("setupDefaultAdminConcurrency()=%d, want %d", got, simpleModeAdminConcurrency)
		}
	})

	t.Run("standard mode keeps existing default", func(t *testing.T) {
		t.Setenv("RUN_MODE", "standard")
		if got := setupDefaultAdminConcurrency(); got != defaultUserConcurrency {
			t.Fatalf("setupDefaultAdminConcurrency()=%d, want %d", got, defaultUserConcurrency)
		}
	})
}

func TestSetupMigrationTimeout(t *testing.T) {
	t.Run("uses default timeout when unset", func(t *testing.T) {
		cfg := &SetupConfig{}
		if got := cfg.migrationTimeout(); got != 60*time.Second {
			t.Fatalf("migrationTimeout()=%s, want 60s", got)
		}
	})

	t.Run("uses configured timeout", func(t *testing.T) {
		cfg := &SetupConfig{MigrationTimeoutSeconds: 300}
		if got := cfg.migrationTimeout(); got != 300*time.Second {
			t.Fatalf("migrationTimeout()=%s, want 300s", got)
		}
	})
}

func TestWriteConfigFileKeepsDefaultUserConcurrency(t *testing.T) {
	t.Setenv("RUN_MODE", "simple")
	t.Setenv("DATA_DIR", t.TempDir())

	if err := writeConfigFile(&SetupConfig{}); err != nil {
		t.Fatalf("writeConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(GetConfigFilePath())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !strings.Contains(string(data), "user_concurrency: 5") {
		t.Fatalf("config missing default user concurrency, got:\n%s", string(data))
	}
}

func TestWriteConfigFileIncludesRedisUsername(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())

	if err := writeConfigFile(&SetupConfig{
		Redis: RedisConfig{
			Host:     "redis",
			Port:     6379,
			Username: "app-user",
		},
	}); err != nil {
		t.Fatalf("writeConfigFile() error = %v", err)
	}

	data, err := os.ReadFile(GetConfigFilePath())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if !strings.Contains(string(data), "username: app-user") {
		t.Fatalf("config missing Redis username, got:\n%s", string(data))
	}
}

func TestBuildDatabaseConnectionDSNsUsesPostgresForBootstrap(t *testing.T) {
	cfg := &DatabaseConfig{
		Host:     "db",
		Port:     5432,
		User:     "sub2api",
		Password: "secret",
		DBName:   "sub2api",
		SSLMode:  "disable",
	}

	bootstrapDSN, targetDSN := buildDatabaseConnectionDSNs(cfg)

	if !strings.Contains(bootstrapDSN, "dbname=postgres") {
		t.Fatalf("bootstrap DSN = %q, want default postgres database", bootstrapDSN)
	}
	if strings.Contains(bootstrapDSN, "dbname=sub2api") {
		t.Fatalf("bootstrap DSN = %q, should not connect to target database before checking/creating it", bootstrapDSN)
	}
	if !strings.Contains(targetDSN, "dbname=sub2api") {
		t.Fatalf("target DSN = %q, want configured database", targetDSN)
	}
}

func TestSetupConfigFromEnv(t *testing.T) {
	t.Setenv("DATABASE_HOST", "migration-postgres")
	t.Setenv("DATABASE_PORT", "55432")
	t.Setenv("DATABASE_USER", "migration-user")
	t.Setenv("DATABASE_PASSWORD", "migration-password")
	t.Setenv("DATABASE_DBNAME", "migration-db")
	t.Setenv("DATABASE_SSLMODE", "require")
	t.Setenv("REDIS_HOST", "migration-redis")
	t.Setenv("REDIS_USERNAME", "redis-user")
	t.Setenv("REDIS_ENABLE_TLS", "true")
	t.Setenv("TZ", "UTC")
	t.Setenv("SETUP_MIGRATION_TIMEOUT_SECONDS", "600")

	cfg := setupConfigFromEnv()
	if cfg.Database.Host != "migration-postgres" || cfg.Database.Port != 55432 {
		t.Fatalf("database address = %s:%d", cfg.Database.Host, cfg.Database.Port)
	}
	if cfg.Database.User != "migration-user" || cfg.Database.Password != "migration-password" {
		t.Fatalf("database credentials were not loaded from environment")
	}
	if cfg.Database.DBName != "migration-db" || cfg.Database.SSLMode != "require" {
		t.Fatalf("database target = %s sslmode=%s", cfg.Database.DBName, cfg.Database.SSLMode)
	}
	if cfg.Redis.Host != "migration-redis" || cfg.Redis.Username != "redis-user" || !cfg.Redis.EnableTLS {
		t.Fatalf("redis configuration was not loaded from environment")
	}
	if cfg.Timezone != "UTC" || cfg.MigrationTimeoutSeconds != 600 {
		t.Fatalf("timezone=%s migration timeout=%d", cfg.Timezone, cfg.MigrationTimeoutSeconds)
	}
}

func TestDatabaseConfigFromEnvDoesNotDependOnServiceConfiguration(t *testing.T) {
	t.Setenv("DATABASE_HOST", "postgres-green")
	t.Setenv("DATABASE_PORT", "55436")
	t.Setenv("DATABASE_USER", "modelport")
	t.Setenv("DATABASE_PASSWORD", "database-secret")
	t.Setenv("DATABASE_DBNAME", "modelport")
	t.Setenv("DATABASE_SSLMODE", "verify-full")
	t.Setenv("REDIS_HOST", "unreachable-redis")
	t.Setenv("ADMIN_EMAIL", "must-not-be-used@example.com")
	t.Setenv("JWT_SECRET", "must-not-be-used")

	cfg := databaseConfigFromEnv()
	if cfg.Host != "postgres-green" || cfg.Port != 55436 {
		t.Fatalf("database address = %s:%d", cfg.Host, cfg.Port)
	}
	if cfg.User != "modelport" || cfg.Password != "database-secret" {
		t.Fatalf("database credentials were not loaded from environment")
	}
	if cfg.DBName != "modelport" || cfg.SSLMode != "verify-full" {
		t.Fatalf("database target = %s sslmode=%s", cfg.DBName, cfg.SSLMode)
	}
}
