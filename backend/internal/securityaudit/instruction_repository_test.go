package securityaudit

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestInstructionRepositoryListEventsAppliesLegacyUserAndModelFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	filterClause := `(?s).*\$11 = 0 OR e\.user_id = \$11.*\$12 = '%%' OR e\.model ILIKE \$12.*`
	args := []driver.Value{
		"", "%%", nil, nil,
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), int64(42), "%gpt-5%",
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*)") + filterClause).
		WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery(`(?s)SELECT e\.id.*` + filterClause + `.*LIMIT \$13 OFFSET \$14`).
		WithArgs(append(args, 20, 0)...).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "request_id", "user_id", "user_email_snapshot", "api_key_id", "group_id",
			"group_name_snapshot", "model", "endpoint", "stage", "instructions_present",
			"instructions_sha256", "instructions_result", "input1_present", "input1_sha256",
			"input1_result", "decision", "reason", "rule_set_ids", "config_version", "latency_ms",
			"evidence_status", "evidence_expires_at", "user_notification_status",
			"ops_notification_status", "created_at",
		}).AddRow(
			int64(1), "request-1", int64(42), "user@example.test", nil, int64(7),
			"test-group", "gpt-5", "/v1/responses", "http", true, "", "mismatch",
			false, "", "missing", "blocked", "hash_mismatch", []byte("[]"), int64(1), 2,
			"stored", nil, "pending", "sent", time.Now().UTC(),
		))

	page, err := NewInstructionRepository(db).ListEvents(context.Background(), 1, 20, InstructionEventFilter{
		UserID: 42,
		Model:  "gpt-5",
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, page.Total)
	require.Len(t, page.Items, 1)
	require.EqualValues(t, 42, *page.Items[0].UserID)
	require.NoError(t, mock.ExpectationsWereMet())
}
