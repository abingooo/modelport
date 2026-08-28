CREATE TEMP TABLE instruction_audit_aggregate_repartition ON COMMIT DROP AS
WITH affected AS (
    SELECT DISTINCT
        bucket_at, user_id, group_id, model, client_type, final_outcome, final_reason
    FROM instruction_audit_outcome_hourly
    WHERE shard_no = -1
), matched AS (
    SELECT h.*
    FROM instruction_audit_outcome_hourly h
    JOIN affected a
      ON h.bucket_at = a.bucket_at
     AND h.user_id = a.user_id
     AND h.group_id = a.group_id
     AND h.model = a.model
     AND h.client_type = a.client_type
     AND h.final_outcome = a.final_outcome
     AND h.final_reason = a.final_reason
), metrics AS (
    SELECT
        bucket_at, user_id, group_id, model, client_type, final_outcome, final_reason,
        SUM(event_count)::BIGINT AS event_count,
        SUM(latency_total_ms)::BIGINT AS latency_total_ms,
        SUM(ai_latency_total_ms)::BIGINT AS ai_latency_total_ms,
        MAX(updated_at) AS updated_at
    FROM matched
    GROUP BY bucket_at, user_id, group_id, model, client_type, final_outcome, final_reason
), event_series AS (
    SELECT
        h.bucket_at, h.user_id, h.group_id, h.model, h.client_type,
        h.final_outcome, h.final_reason,
        ARRAY_AGG(item.event_time ORDER BY item.event_time, h.shard_no, item.ordinality) AS event_times
    FROM matched h
    CROSS JOIN LATERAL UNNEST(h.event_times) WITH ORDINALITY AS item(event_time, ordinality)
    GROUP BY h.bucket_at, h.user_id, h.group_id, h.model, h.client_type,
        h.final_outcome, h.final_reason
)
SELECT
    m.bucket_at, m.user_id, m.group_id, m.model, m.client_type,
    m.final_outcome, m.final_reason, m.event_count, m.latency_total_ms,
    m.ai_latency_total_ms, e.event_times, m.updated_at
FROM metrics m
JOIN event_series e
  ON m.bucket_at = e.bucket_at
 AND m.user_id = e.user_id
 AND m.group_id = e.group_id
 AND m.model = e.model
 AND m.client_type = e.client_type
 AND m.final_outcome = e.final_outcome
 AND m.final_reason = e.final_reason;

DELETE FROM instruction_audit_outcome_hourly h
USING instruction_audit_aggregate_repartition r
WHERE h.bucket_at = r.bucket_at
  AND h.user_id = r.user_id
  AND h.group_id = r.group_id
  AND h.model = r.model
  AND h.client_type = r.client_type
  AND h.final_outcome = r.final_outcome
  AND h.final_reason = r.final_reason;

WITH expanded AS (
    SELECT
        r.*,
        generated.shard_no,
        (generated.shard_no * 4096)::INT AS previous_events,
        LEAST(
            ((generated.shard_no + 1) * 4096)::INT,
            cardinality(r.event_times)
        ) AS cumulative_events
    FROM instruction_audit_aggregate_repartition r
    CROSS JOIN LATERAL generate_series(
        0::BIGINT,
        ((cardinality(r.event_times) - 1) / 4096)::BIGINT
    ) AS generated(shard_no)
), sliced AS (
    SELECT
        expanded.*,
        event_times[(previous_events + 1):cumulative_events] AS shard_event_times
    FROM expanded
)
INSERT INTO instruction_audit_outcome_hourly (
    bucket_at, user_id, group_id, model, client_type, final_outcome, final_reason,
    shard_no, event_count, latency_total_ms, ai_latency_total_ms, event_times,
    first_event_at, last_event_at, updated_at
)
SELECT
    bucket_at, user_id, group_id, model, client_type, final_outcome, final_reason,
    shard_no,
    (
        FLOOR(event_count::NUMERIC * cumulative_events / cardinality(event_times))
        - FLOOR(event_count::NUMERIC * previous_events / cardinality(event_times))
    )::BIGINT,
    (
        FLOOR(latency_total_ms::NUMERIC * cumulative_events / cardinality(event_times))
        - FLOOR(latency_total_ms::NUMERIC * previous_events / cardinality(event_times))
    )::BIGINT,
    (
        FLOOR(ai_latency_total_ms::NUMERIC * cumulative_events / cardinality(event_times))
        - FLOOR(ai_latency_total_ms::NUMERIC * previous_events / cardinality(event_times))
    )::BIGINT,
    shard_event_times,
    shard_event_times[1],
    shard_event_times[cardinality(shard_event_times)],
    updated_at
FROM sliced;

ALTER TABLE instruction_audit_outcome_hourly
    DROP CONSTRAINT IF EXISTS chk_instruction_audit_outcome_hourly_shard;
ALTER TABLE instruction_audit_outcome_hourly
    ADD CONSTRAINT chk_instruction_audit_outcome_hourly_shard CHECK (shard_no >= 0);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_instruction_audit_outcome_hourly_event_times_bounded'
          AND conrelid = 'instruction_audit_outcome_hourly'::regclass
    ) THEN
        ALTER TABLE instruction_audit_outcome_hourly
            ADD CONSTRAINT chk_instruction_audit_outcome_hourly_event_times_bounded
            CHECK (cardinality(event_times) <= 4096);
    END IF;
END $$;
