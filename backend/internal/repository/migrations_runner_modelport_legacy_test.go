package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

const legacyModelPortOpenAICompatibleProvidersChecksum = "10d789021cf0b48b5d4c1cc4aab998c60b2ce701196aa0eca56cb5df2d16ee6b"
const legacyModelPortChannelMonitorProvidersChecksum = "55f442b5fc7150becc00b477300e26c379da564b8cfff410dd600eec80942de9"

func TestLegacyModelPortArchivedMigrationChecksumsMatchReleaseManifest(t *testing.T) {
	for name, expected := range map[string]string{
		legacyModelPortOpenAICompatibleProvidersMigration: legacyModelPortOpenAICompatibleProvidersChecksum,
		legacyModelPortChannelMonitorProvidersMigration:   legacyModelPortChannelMonitorProvidersChecksum,
	} {
		actual, err := legacyModelPortArchivedMigrationChecksum(name)
		require.NoError(t, err)
		require.Equal(t, expected, actual)
	}
}

func TestLegacyModelPortUnknownPlatformQueryCoversAccountAndGroupRows(t *testing.T) {
	for _, source := range []string{"accounts.platform", "groups.platform"} {
		require.Contains(t, legacyModelPortUnknownPlatformValuesQuery, "'"+source+"'")
	}
	require.Contains(t, legacyModelPortUnknownPlatformValuesQuery, "FROM accounts")
	require.Contains(t, legacyModelPortUnknownPlatformValuesQuery, "FROM groups")
	// Kiro is a supported generic OpenAI passthrough account value, but it is
	// intentionally absent from the terminal quota/route/provider checks.
	accountsClause := legacyModelPortUnknownPlatformValuesQuery[strings.Index(legacyModelPortUnknownPlatformValuesQuery, "FROM accounts"):strings.Index(legacyModelPortUnknownPlatformValuesQuery, "UNION ALL\n\tSELECT 'groups.platform'")]
	require.Contains(t, accountsClause, "'kiro'")
	require.NotContains(t, legacyModelPortUnknownPlatformValuesQuery[strings.Index(legacyModelPortUnknownPlatformValuesQuery, "'user_platform_quotas.platform'"):], "'kiro'")
}

func TestLegacyModelPortPlatformQueriesSeparateRemovedAndUnknownRows(t *testing.T) {
	for _, table := range []string{"accounts", "groups", "user_platform_quotas", "composite_model_routes"} {
		require.Contains(t, legacyModelPortBlockedPlatformValuesQuery, table)
		require.Contains(t, legacyModelPortUnknownPlatformValuesQuery, table)
	}
	for _, table := range []string{"channel_monitors", "channel_monitor_request_templates"} {
		require.Contains(t, legacyModelPortUnknownPlatformValuesQuery, table)
	}
	require.Contains(t, legacyModelPortBlockedPlatformValuesQuery, "status = 'active'")
	require.Contains(t, legacyModelPortBlockedPlatformValuesQuery, "deleted_at IS NULL")
	require.Contains(t, legacyModelPortBlockedPlatformValuesQuery, "AND enabled")
	require.Contains(t, legacyModelPortBlockedPlatformValuesQuery, "daily_limit_usd IS NOT NULL")
	require.Contains(t, legacyModelPortUnknownPlatformValuesQuery, "platform IS NOT NULL")
}

func TestPrepareLegacyModelPortPlatformConstraintsSkipsNonLegacyDatabase(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectLegacyMigrationAbsent(mock, legacyModelPortOpenAICompatibleProvidersMigration)
	expectLegacyMigrationAbsent(mock, legacyModelPortChannelMonitorProvidersMigration)

	err = prepareLegacyModelPortPlatformConstraints(context.Background(), db, fstest.MapFS{})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrepareLegacyModelPortPlatformConstraintsBridgesConflictingUpstreamMigrations(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	userQuotaSQL := "ALTER TABLE user_platform_quotas DROP CONSTRAINT legacy;"
	compositeSQL := "ALTER TABLE composite_model_routes DROP CONSTRAINT legacy;"
	bridgeSQL := "ALTER TABLE user_platform_quotas DROP CONSTRAINT legacy;"
	fsys := fstest.MapFS{
		upstreamUserPlatformCNProvidersMigration:    &fstest.MapFile{Data: []byte(userQuotaSQL)},
		upstreamCompositeRoutesCNProvidersMigration: &fstest.MapFile{Data: []byte(compositeSQL)},
		modelPortLegacyPlatformConstraintsMigration: &fstest.MapFile{Data: []byte(bridgeSQL)},
	}

	expectLegacyMigrationChecksum(mock, legacyModelPortOpenAICompatibleProvidersMigration, legacyModelPortOpenAICompatibleProvidersChecksum)
	expectLegacyMigrationChecksum(mock, legacyModelPortChannelMonitorProvidersMigration, legacyModelPortChannelMonitorProvidersChecksum)
	expectMigrationAbsent(mock, upstreamUserPlatformCNProvidersMigration)
	expectMigrationAbsent(mock, upstreamChannelMonitorQuotaModeMigration)
	expectMigrationAbsent(mock, upstreamCompositeRoutesCNProvidersMigration)
	expectMigrationAbsent(mock, modelPortLegacyPlatformConstraintsMigration)
	expectEmptyLegacyPlatformQuery(mock)
	mock.ExpectQuery("SELECT source, value").
		WillReturnRows(sqlmock.NewRows([]string{"source", "value"}))
	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE user_platform_quotas").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO schema_migrations").
		WithArgs(upstreamUserPlatformCNProvidersMigration, migrationChecksum(userQuotaSQL)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO schema_migrations").
		WithArgs(upstreamCompositeRoutesCNProvidersMigration, migrationChecksum(compositeSQL)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = prepareLegacyModelPortPlatformConstraints(context.Background(), db, fsys)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrepareLegacyModelPortPlatformConstraintsRejectsUnknownValues(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectLegacyMigrationChecksum(mock, legacyModelPortOpenAICompatibleProvidersMigration, legacyModelPortOpenAICompatibleProvidersChecksum)
	expectLegacyMigrationAbsent(mock, legacyModelPortChannelMonitorProvidersMigration)
	expectMigrationAbsent(mock, upstreamUserPlatformCNProvidersMigration)
	expectMigrationAbsent(mock, upstreamChannelMonitorQuotaModeMigration)
	expectMigrationAbsent(mock, upstreamCompositeRoutesCNProvidersMigration)
	expectMigrationAbsent(mock, modelPortLegacyPlatformConstraintsMigration)
	expectEmptyLegacyPlatformQuery(mock)
	mock.ExpectQuery("SELECT source, value").
		WillReturnRows(sqlmock.NewRows([]string{"source", "value"}).
			AddRow("user_platform_quotas.platform", "future-provider"))

	err = prepareLegacyModelPortPlatformConstraints(context.Background(), db, fstest.MapFS{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "future-provider")
	require.Contains(t, err.Error(), "preserve and classify")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrepareLegacyModelPortPlatformConstraintsPreparesChannelMonitorWithoutSkippingMigration226(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	userQuotaSQL := "ALTER TABLE user_platform_quotas DROP CONSTRAINT legacy;"
	compositeSQL := "ALTER TABLE composite_model_routes DROP CONSTRAINT legacy;"
	bridgeSQL := "ALTER TABLE channel_monitors DROP CONSTRAINT legacy;"
	fsys := fstest.MapFS{
		upstreamUserPlatformCNProvidersMigration:    &fstest.MapFile{Data: []byte(userQuotaSQL)},
		upstreamCompositeRoutesCNProvidersMigration: &fstest.MapFile{Data: []byte(compositeSQL)},
		modelPortLegacyPlatformConstraintsMigration: &fstest.MapFile{Data: []byte(bridgeSQL)},
	}
	expectLegacyMigrationAbsent(mock, legacyModelPortOpenAICompatibleProvidersMigration)
	expectLegacyMigrationChecksum(mock, legacyModelPortChannelMonitorProvidersMigration, legacyModelPortChannelMonitorProvidersChecksum)
	expectMigrationAbsent(mock, upstreamUserPlatformCNProvidersMigration)
	expectMigrationAbsent(mock, upstreamChannelMonitorQuotaModeMigration)
	expectMigrationAbsent(mock, upstreamCompositeRoutesCNProvidersMigration)
	expectMigrationAbsent(mock, modelPortLegacyPlatformConstraintsMigration)
	// Empty blocked/unknown results model storage-only removed values: they are
	// intentionally retained by 236, while the constraint-only 224/227 inputs
	// must still be recorded so they cannot narrow those rows later.
	expectEmptyLegacyPlatformQuery(mock)
	mock.ExpectQuery("SELECT source, value").
		WillReturnRows(sqlmock.NewRows([]string{"source", "value"}))
	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE channel_monitors").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO schema_migrations").
		WithArgs(upstreamUserPlatformCNProvidersMigration, migrationChecksum(userQuotaSQL)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO schema_migrations").
		WithArgs(upstreamCompositeRoutesCNProvidersMigration, migrationChecksum(compositeSQL)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = prepareLegacyModelPortPlatformConstraints(context.Background(), db, fsys)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrepareLegacyModelPortPlatformConstraintsRequiresFinalBridgeMigration(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectLegacyMigrationAbsent(mock, legacyModelPortOpenAICompatibleProvidersMigration)
	expectLegacyMigrationChecksum(mock, legacyModelPortChannelMonitorProvidersMigration, legacyModelPortChannelMonitorProvidersChecksum)
	expectMigrationAbsent(mock, upstreamUserPlatformCNProvidersMigration)
	expectMigrationAbsent(mock, upstreamChannelMonitorQuotaModeMigration)
	expectMigrationAbsent(mock, upstreamCompositeRoutesCNProvidersMigration)
	expectMigrationAbsent(mock, modelPortLegacyPlatformConstraintsMigration)
	expectEmptyLegacyPlatformQuery(mock)
	mock.ExpectQuery("SELECT source, value").
		WillReturnRows(sqlmock.NewRows([]string{"source", "value"}))

	err = prepareLegacyModelPortPlatformConstraints(context.Background(), db, fstest.MapFS{})
	require.Error(t, err)
	require.Contains(t, err.Error(), modelPortLegacyPlatformConstraintsMigration)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrepareLegacyModelPortPlatformConstraintsSkipsCompletedBridgeInputs(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	expectLegacyMigrationChecksum(mock, legacyModelPortOpenAICompatibleProvidersMigration, legacyModelPortOpenAICompatibleProvidersChecksum)
	expectLegacyMigrationChecksum(mock, legacyModelPortChannelMonitorProvidersMigration, legacyModelPortChannelMonitorProvidersChecksum)
	userQuotaSQL := "ALTER TABLE user_platform_quotas DROP CONSTRAINT legacy;"
	channelMonitorSQL := "ALTER TABLE channel_monitors ADD COLUMN check_mode text;"
	compositeSQL := "ALTER TABLE composite_model_routes DROP CONSTRAINT legacy;"
	fsys := fstest.MapFS{
		upstreamUserPlatformCNProvidersMigration:    &fstest.MapFile{Data: []byte(userQuotaSQL)},
		upstreamChannelMonitorQuotaModeMigration:    &fstest.MapFile{Data: []byte(channelMonitorSQL)},
		upstreamCompositeRoutesCNProvidersMigration: &fstest.MapFile{Data: []byte(compositeSQL)},
	}
	expectMigrationChecksum(mock, upstreamUserPlatformCNProvidersMigration, migrationChecksum(userQuotaSQL))
	expectMigrationChecksum(mock, upstreamChannelMonitorQuotaModeMigration, migrationChecksum(channelMonitorSQL))
	expectMigrationChecksum(mock, upstreamCompositeRoutesCNProvidersMigration, migrationChecksum(compositeSQL))
	expectMigrationAbsent(mock, modelPortLegacyPlatformConstraintsMigration)

	err = prepareLegacyModelPortPlatformConstraints(context.Background(), db, fsys)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPrepareLegacyModelPortPlatformConstraintsRejectsBadUpstreamChecksumsBeforeBridge(t *testing.T) {
	tests := []struct {
		name      string
		migration string
		preceding []string
	}{
		{
			name:      "224 partially applied",
			migration: upstreamUserPlatformCNProvidersMigration,
		},
		{
			name:      "226 partially applied",
			migration: upstreamChannelMonitorQuotaModeMigration,
			preceding: []string{upstreamUserPlatformCNProvidersMigration},
		},
		{
			name:      "227 partially applied",
			migration: upstreamCompositeRoutesCNProvidersMigration,
			preceding: []string{
				upstreamUserPlatformCNProvidersMigration,
				upstreamChannelMonitorQuotaModeMigration,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			migrationSQL := "SELECT '" + tt.migration + "';"
			fsys := fstest.MapFS{
				tt.migration: &fstest.MapFile{Data: []byte(migrationSQL)},
			}
			expectLegacyMigrationChecksum(mock, legacyModelPortOpenAICompatibleProvidersMigration, legacyModelPortOpenAICompatibleProvidersChecksum)
			expectLegacyMigrationChecksum(mock, legacyModelPortChannelMonitorProvidersMigration, legacyModelPortChannelMonitorProvidersChecksum)
			for _, name := range tt.preceding {
				expectMigrationAbsent(mock, name)
			}
			expectMigrationChecksum(mock, tt.migration, "wrong-checksum")

			err = prepareLegacyModelPortPlatformConstraints(context.Background(), db, fsys)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.migration)
			require.Contains(t, err.Error(), "checksum mismatch")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestPrepareLegacyModelPortPlatformConstraintsRejectsUnrecognizedLedgerChecksums(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(sqlmock.Sqlmock)
		marker string
	}{
		{
			name: "quota marker",
			setup: func(mock sqlmock.Sqlmock) {
				expectLegacyMigrationChecksum(mock, legacyModelPortOpenAICompatibleProvidersMigration, "wrong-checksum")
			},
			marker: legacyModelPortOpenAICompatibleProvidersMigration,
		},
		{
			name: "monitor marker",
			setup: func(mock sqlmock.Sqlmock) {
				expectLegacyMigrationAbsent(mock, legacyModelPortOpenAICompatibleProvidersMigration)
				expectLegacyMigrationChecksum(mock, legacyModelPortChannelMonitorProvidersMigration, "wrong-checksum")
			},
			marker: legacyModelPortChannelMonitorProvidersMigration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()

			tt.setup(mock)
			err = prepareLegacyModelPortPlatformConstraints(context.Background(), db, fstest.MapFS{})
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.marker)
			require.Contains(t, err.Error(), "checksum mismatch")
			require.Contains(t, err.Error(), "database identity is ambiguous")
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestApplyLegacyModelPortPlatformBridgeRollsBackWhenDeferredMigrationFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	const bridgeSQL = "ALTER TABLE user_platform_quotas DROP CONSTRAINT legacy;"
	const channelMonitorSQL = "ALTER TABLE channel_monitors ADD COLUMN check_mode text;"
	plan := &legacyModelPortPlatformBridgePlan{
		deferredMigrations: map[string]struct{}{
			modelPortLegacyPlatformConstraintsMigration: {},
			upstreamChannelMonitorQuotaModeMigration:    {},
		},
		equivalentChecksums: map[string]string{
			upstreamUserPlatformCNProvidersMigration: "quota-checksum",
		},
		bridgeContent:  bridgeSQL,
		bridgeChecksum: "bridge-checksum",
	}
	fsys := fstest.MapFS{
		upstreamChannelMonitorQuotaModeMigration: &fstest.MapFile{Data: []byte(channelMonitorSQL)},
	}

	mock.ExpectBegin()
	mock.ExpectExec("ALTER TABLE user_platform_quotas").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("ALTER TABLE channel_monitors").
		WillReturnError(errors.New("deferred migration failed"))
	mock.ExpectRollback()

	err = applyLegacyModelPortPlatformBridge(context.Background(), db, fsys, plan)
	require.Error(t, err)
	require.Contains(t, err.Error(), "deferred migration")
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectLegacyMigrationAbsent(mock sqlmock.Sqlmock, name string) {
	mock.ExpectQuery("SELECT checksum FROM schema_migrations").
		WithArgs(name).
		WillReturnError(sql.ErrNoRows)
}

func expectLegacyMigrationChecksum(mock sqlmock.Sqlmock, name, checksum string) {
	mock.ExpectQuery("SELECT checksum FROM schema_migrations").
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(checksum))
}

func expectMigrationAbsent(mock sqlmock.Sqlmock, name string) {
	mock.ExpectQuery("SELECT checksum FROM schema_migrations").
		WithArgs(name).
		WillReturnError(sql.ErrNoRows)
}

func expectMigrationChecksum(mock sqlmock.Sqlmock, name, checksum string) {
	mock.ExpectQuery("SELECT checksum FROM schema_migrations").
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(checksum))
}

func expectEmptyLegacyPlatformQuery(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT source, value").
		WillReturnRows(sqlmock.NewRows([]string{"source", "value"}))
}
