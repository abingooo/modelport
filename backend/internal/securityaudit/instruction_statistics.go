package securityaudit

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/lib/pq"
)

const instructionOutcomeAggregateShardSize int64 = 4096

func (r *InstructionRepository) ArchiveExpiredPassEvents(ctx context.Context, retentionDays int, batchSize int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("instruction audit repository unavailable")
	}
	if retentionDays < 1 || retentionDays > 90 {
		return 0, errors.New("instruction audit pass retention is invalid")
	}
	if batchSize < 1 || batchSize > 5000 {
		batchSize = 500
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var archived, lastArchivedEventID int64
	err = tx.QueryRowContext(ctx, `
		WITH candidates AS MATERIALIZED (
			SELECT e.*
			FROM instruction_audit_events e
			WHERE e.final_outcome IN ('hash_pass', 'exception_pass')
			  AND e.created_at < date_trunc('hour', NOW() - make_interval(days => $1))
			ORDER BY e.id
			LIMIT $2
			FOR UPDATE OF e SKIP LOCKED
		), archived AS (
			INSERT INTO instruction_audit_outcome_hourly
					(bucket_at, user_id, group_id, model, client_type, final_outcome, final_reason,
					 shard_no, event_count, latency_total_ms, ai_latency_total_ms, event_times,
					 first_event_at, last_event_at)
				SELECT date_trunc('hour', created_at), COALESCE(user_id, 0), COALESCE(group_id, 0),
					model, client_type, final_outcome, final_reason, id / $3, COUNT(*), SUM(latency_ms),
					SUM(COALESCE(ai_latency_ms, 0)), ARRAY_AGG(created_at ORDER BY created_at),
					MIN(created_at), MAX(created_at)
				FROM candidates
				GROUP BY date_trunc('hour', created_at), COALESCE(user_id, 0), COALESCE(group_id, 0),
					model, client_type, final_outcome, final_reason, id / $3
				ON CONFLICT (bucket_at, user_id, group_id, model, client_type, final_outcome, final_reason, shard_no)
			DO UPDATE SET
				event_count = instruction_audit_outcome_hourly.event_count + EXCLUDED.event_count,
				latency_total_ms = instruction_audit_outcome_hourly.latency_total_ms + EXCLUDED.latency_total_ms,
				ai_latency_total_ms = instruction_audit_outcome_hourly.ai_latency_total_ms + EXCLUDED.ai_latency_total_ms,
				event_times = instruction_audit_outcome_hourly.event_times || EXCLUDED.event_times,
				first_event_at = LEAST(instruction_audit_outcome_hourly.first_event_at, EXCLUDED.first_event_at),
				last_event_at = GREATEST(instruction_audit_outcome_hourly.last_event_at, EXCLUDED.last_event_at),
				updated_at = NOW()
			RETURNING 1
		), deleted AS (
			DELETE FROM instruction_audit_events e
			USING candidates c
			WHERE e.id = c.id
			RETURNING e.id
		)
			SELECT COUNT(*), COALESCE(MAX(id), 0) FROM deleted`, retentionDays, batchSize, instructionOutcomeAggregateShardSize).Scan(&archived, &lastArchivedEventID)
	if err != nil {
		return 0, err
	}
	if archived > 0 {
		if _, err = tx.ExecContext(ctx, `
			UPDATE instruction_audit_outcome_rollup_state
			SET last_event_id = GREATEST(last_event_id, $1), updated_at = NOW()
			WHERE id = 1`, lastArchivedEventID); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return archived, nil
}

func (r *InstructionRepository) InstructionStatistics(ctx context.Context, filter InstructionEventFilter) (*InstructionStatistics, error) {
	filter = canonicalInstructionEventFilter(filter)
	rows, err := r.db.QueryContext(ctx, `
		WITH outcome_counts AS (
			SELECT h.final_outcome,
				SUM(
					CASE
						WHEN ($1::TIMESTAMPTZ IS NULL OR $1::TIMESTAMPTZ <= h.bucket_at)
						 AND ($2::TIMESTAMPTZ IS NULL OR $2::TIMESTAMPTZ >= h.bucket_at + INTERVAL '1 hour')
						THEN h.event_count
						ELSE (
							SELECT COUNT(*)
							FROM UNNEST(h.event_times) AS archived_event(created_at)
							WHERE ($1::TIMESTAMPTZ IS NULL OR archived_event.created_at >= $1::TIMESTAMPTZ)
							  AND ($2::TIMESTAMPTZ IS NULL OR archived_event.created_at < $2::TIMESTAMPTZ)
						)
					END
				)::BIGINT AS event_count
			FROM instruction_audit_outcome_hourly h
			WHERE ($1::TIMESTAMPTZ IS NULL OR h.last_event_at >= $1::TIMESTAMPTZ)
			  AND ($2::TIMESTAMPTZ IS NULL OR h.first_event_at < $2::TIMESTAMPTZ)
			  AND (cardinality($3::BIGINT[]) = 0 OR h.group_id = ANY($3::BIGINT[]))
			  AND ($4 = 0 OR h.user_id = $4)
			  AND ($5 = '' OR h.model ILIKE $5)
			  AND (cardinality($6::TEXT[]) = 0 OR h.client_type = ANY($6::TEXT[]))
			  AND (cardinality($7::TEXT[]) = 0 OR h.final_outcome = ANY($7::TEXT[]))
			  AND (cardinality($8::TEXT[]) = 0 OR h.final_reason = ANY($8::TEXT[]))
			GROUP BY h.final_outcome
			UNION ALL
			SELECT e.final_outcome, COUNT(*)::BIGINT
			FROM instruction_audit_events e
			WHERE ($1::TIMESTAMPTZ IS NULL OR e.created_at >= $1::TIMESTAMPTZ)
			  AND ($2::TIMESTAMPTZ IS NULL OR e.created_at < $2::TIMESTAMPTZ)
			  AND (cardinality($3::BIGINT[]) = 0 OR e.group_id = ANY($3::BIGINT[]))
			  AND ($4 = 0 OR e.user_id = $4)
			  AND ($5 = '' OR e.model ILIKE $5)
			  AND (cardinality($6::TEXT[]) = 0 OR e.client_type = ANY($6::TEXT[]))
			  AND (cardinality($7::TEXT[]) = 0 OR e.final_outcome = ANY($7::TEXT[]))
			  AND (cardinality($8::TEXT[]) = 0 OR e.final_reason = ANY($8::TEXT[]))
			GROUP BY e.final_outcome
		)
		SELECT final_outcome, SUM(event_count)::BIGINT
		FROM outcome_counts GROUP BY final_outcome`,
		filter.From, filter.To, pq.Array(filter.GroupIDs), filter.UserID,
		instructionStatisticsModelPattern(filter.Model), pq.Array(filter.ClientTypes),
		pq.Array(filter.Outcomes), pq.Array(filter.FinalReasons))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	statistics := &InstructionStatistics{}
	for rows.Next() {
		var outcome string
		var count int64
		if err := rows.Scan(&outcome, &count); err != nil {
			return nil, err
		}
		switch outcome {
		case InstructionOutcomeBlocked:
			statistics.Blocked = count
		case InstructionOutcomePolicyAllow:
			statistics.PolicyAllow = count
		case InstructionOutcomeAIPass:
			statistics.AIPass = count
		case InstructionOutcomeHashPass:
			statistics.HashPass = count
		case InstructionOutcomeExceptionPass:
			statistics.ExceptionPass = count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	statistics.Total = statistics.Blocked + statistics.PolicyAllow + statistics.AIPass + statistics.HashPass + statistics.ExceptionPass
	if statistics.Total > 0 {
		statistics.BlockRate = float64(statistics.Blocked) / float64(statistics.Total)
	}
	return statistics, nil
}

func (r *InstructionRepository) InstructionLatencyMetrics(ctx context.Context, since time.Time) (InstructionLatencyMetrics, error) {
	var metrics InstructionLatencyMetrics
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE ai_latency_ms IS NULL),
			COALESCE(percentile_disc(0.95) WITHIN GROUP (ORDER BY latency_ms)
				FILTER (WHERE ai_latency_ms IS NULL), 0),
			COALESCE(percentile_disc(0.99) WITHIN GROUP (ORDER BY latency_ms)
				FILTER (WHERE ai_latency_ms IS NULL), 0),
			COUNT(*) FILTER (WHERE ai_latency_ms IS NOT NULL),
			COALESCE(percentile_disc(0.95) WITHIN GROUP (ORDER BY ai_latency_ms)
				FILTER (WHERE ai_latency_ms IS NOT NULL), 0),
			COALESCE(percentile_disc(0.99) WITHIN GROUP (ORDER BY ai_latency_ms)
				FILTER (WHERE ai_latency_ms IS NOT NULL), 0)
		FROM instruction_audit_events
		WHERE created_at >= $1`, since.UTC()).Scan(
		&metrics.AuditSampleCount, &metrics.AuditP95MS, &metrics.AuditP99MS,
		&metrics.AISampleCount, &metrics.AIP95MS, &metrics.AIP99MS,
	)
	return metrics, err
}

func instructionStatisticsModelPattern(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	return "%" + model + "%"
}

func (r *InstructionRepository) InstructionOutcomeStorageCounts(ctx context.Context) (int64, int64, error) {
	var persisted, aggregated sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM instruction_audit_events),
			(SELECT COALESCE(SUM(event_count), 0) FROM instruction_audit_outcome_hourly)`).Scan(&persisted, &aggregated)
	return persisted.Int64, aggregated.Int64, err
}

func instructionPassRetention(snapshot *instructionSnapshot) int {
	if snapshot == nil || snapshot.Runtime.PassEventRetentionDays < 1 {
		return 7
	}
	return snapshot.Runtime.PassEventRetentionDays
}

func instructionAggregateRetention(snapshot *instructionSnapshot) int {
	if snapshot == nil || snapshot.Runtime.AggregateRetentionDays < 30 {
		return 365
	}
	return snapshot.Runtime.AggregateRetentionDays
}

func (r *InstructionRepository) PruneExpiredOutcomeAggregates(
	ctx context.Context,
	retentionDays int,
	batchSize int,
) (int64, int64, error) {
	if r == nil || r.db == nil {
		return 0, 0, errors.New("instruction audit repository unavailable")
	}
	if retentionDays < 30 || retentionDays > 3650 {
		return 0, 0, errors.New("instruction audit aggregate retention is invalid")
	}
	if batchSize < 1 || batchSize > 5000 {
		batchSize = 500
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = tx.Rollback() }()
	var deletedRows, deletedEvents int64
	err = tx.QueryRowContext(ctx, `
		WITH candidates AS MATERIALIZED (
			SELECT ctid
			FROM instruction_audit_outcome_hourly
			WHERE bucket_at < date_trunc('hour', NOW() - make_interval(days => $1))
			ORDER BY bucket_at
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		), deleted AS (
			DELETE FROM instruction_audit_outcome_hourly h
			USING candidates c
			WHERE h.ctid = c.ctid
			RETURNING h.event_count
		)
		SELECT COUNT(*), COALESCE(SUM(event_count), 0) FROM deleted`, retentionDays, batchSize).Scan(
		&deletedRows, &deletedEvents,
	)
	if err != nil {
		return 0, 0, err
	}
	if deletedEvents > 0 {
		if _, err = tx.ExecContext(ctx, `
			UPDATE instruction_audit_outcome_rollup_state
			SET expired_aggregate_event_count = expired_aggregate_event_count + $1,
				last_aggregate_pruned_at = NOW(), updated_at = NOW()
			WHERE id = 1`, deletedEvents); err != nil {
			return 0, 0, err
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return deletedRows, deletedEvents, nil
}

func (r *InstructionRepository) ExpiredAggregateEventCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRowContext(ctx, `
		SELECT expired_aggregate_event_count
		FROM instruction_audit_outcome_rollup_state WHERE id = 1`).Scan(&count)
	return count, err
}

func instructionStatisticsDefaultRange(now time.Time) (*time.Time, *time.Time) {
	to := now.UTC()
	from := to.Add(-24 * time.Hour)
	return &from, &to
}
