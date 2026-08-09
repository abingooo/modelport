package repository

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestBuildContentModerationLogWhere_BlockedIncludesAllBlockActions(t *testing.T) {
	where, args := buildContentModerationLogWhere(service.ContentModerationLogFilter{Result: "blocked"})

	require.Empty(t, args)
	sql := strings.Join(where, " AND ")
	require.Contains(t, sql, "l.action IN ('block', 'keyword_block', 'hash_block')")
	require.NotContains(t, sql, "l.action = 'block'")
}

func TestContentModerationRepositoryCountFlaggedByUserSince_ExcludesHashBlock(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	since := time.Now().Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("AND action <> 'hash_block'")).
		WithArgs(int64(1001), since, false).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	count, err := repo.CountFlaggedByUserSince(context.Background(), 1001, since, false)

	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryCountFlaggedByUserSince_ExcludesCyberPolicyWhenRequested(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	since := time.Now().Add(-time.Hour)
	mock.ExpectQuery(regexp.QuoteMeta("AND ($3::bool IS FALSE OR action <> 'cyber_policy')")).
		WithArgs(int64(1001), since, true).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	count, err := repo.CountFlaggedByUserSince(context.Background(), 1001, since, true)

	require.NoError(t, err)
	require.Equal(t, 3, count)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryCreateLogPersistsCyberEvidenceInTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	createdAt := time.Now()
	log := &service.ContentModerationLog{
		RequestID: "req-cyber", Action: service.ContentModerationActionCyberPolicy,
		CyberEvidence: &service.ContentModerationCyberEvidence{
			RequestBodyCiphertext: "encrypted-body",
			RequestBodySHA256:     strings.Repeat("a", 64),
			RequestBodyBytes:      128,
			EncryptionVersion:     "aes-256-gcm-v1",
		},
	}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT INTO content_moderation_logs").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(42), createdAt))
	mock.ExpectExec("INSERT INTO content_moderation_cyber_evidence").
		WithArgs(int64(42), "encrypted-body", strings.Repeat("a", 64), int64(128), "aes-256-gcm-v1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	err = repo.CreateLog(context.Background(), log)

	require.NoError(t, err)
	require.True(t, log.CyberEvidenceAvailable)
	require.Equal(t, strings.Repeat("a", 64), log.CyberEvidenceSHA256)
	require.Equal(t, int64(128), log.CyberEvidenceBytes)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestContentModerationRepositoryGetCyberPolicyEvidence(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	repo := NewContentModerationRepository(db)
	createdAt := time.Now()
	mock.ExpectQuery("SELECT e.log_id, e.request_body_ciphertext").
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{
			"log_id", "request_body_ciphertext", "request_body_sha256", "request_body_bytes", "encryption_version", "created_at",
		}).AddRow(int64(42), "ciphertext", strings.Repeat("b", 64), int64(256), "aes-256-gcm-v1", createdAt))

	evidenceRepo, ok := repo.(service.ContentModerationCyberEvidenceRepository)
	require.True(t, ok)
	evidence, err := evidenceRepo.GetCyberPolicyEvidence(context.Background(), 42)

	require.NoError(t, err)
	require.Equal(t, int64(42), evidence.LogID)
	require.Equal(t, "ciphertext", evidence.RequestBodyCiphertext)
	require.Equal(t, int64(256), evidence.RequestBodyBytes)
	require.NoError(t, mock.ExpectationsWereMet())
}
