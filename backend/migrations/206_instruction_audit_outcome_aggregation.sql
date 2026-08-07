CREATE TABLE IF NOT EXISTS instruction_audit_outcome_hourly (
    bucket_at             TIMESTAMPTZ NOT NULL,
    user_id               BIGINT NOT NULL DEFAULT 0,
    group_id              BIGINT NOT NULL DEFAULT 0,
    model                 VARCHAR(255) NOT NULL DEFAULT '',
    client_type           VARCHAR(32) NOT NULL DEFAULT 'unknown',
    final_outcome         VARCHAR(32) NOT NULL,
    final_reason          VARCHAR(64) NOT NULL DEFAULT '',
    event_count           BIGINT NOT NULL DEFAULT 0,
    latency_total_ms      BIGINT NOT NULL DEFAULT 0,
    ai_latency_total_ms   BIGINT NOT NULL DEFAULT 0,
    event_times           TIMESTAMPTZ[] NOT NULL DEFAULT ARRAY[]::TIMESTAMPTZ[],
    first_event_at        TIMESTAMPTZ NOT NULL,
    last_event_at         TIMESTAMPTZ NOT NULL,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (
        bucket_at, user_id, group_id, model, client_type, final_outcome, final_reason
    ),
    CONSTRAINT chk_instruction_audit_outcome_hourly_outcome CHECK (
        final_outcome IN ('blocked', 'policy_allow', 'ai_pass', 'hash_pass', 'exception_pass')
    ),
    CONSTRAINT chk_instruction_audit_outcome_hourly_values CHECK (
        event_count >= 0 AND latency_total_ms >= 0 AND ai_latency_total_ms >= 0
    )
);

ALTER TABLE instruction_audit_outcome_hourly
    ADD COLUMN IF NOT EXISTS event_times TIMESTAMPTZ[] NOT NULL DEFAULT ARRAY[]::TIMESTAMPTZ[];

CREATE TABLE IF NOT EXISTS instruction_audit_outcome_rollup_state (
    id                    SMALLINT PRIMARY KEY DEFAULT 1,
    last_event_id         BIGINT NOT NULL DEFAULT 0,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_instruction_audit_rollup_singleton CHECK (id = 1),
    CONSTRAINT chk_instruction_audit_rollup_watermark CHECK (last_event_id >= 0)
);

INSERT INTO instruction_audit_outcome_rollup_state (id, last_event_id)
VALUES (1, 0)
ON CONFLICT (id) DO NOTHING;

CREATE INDEX IF NOT EXISTS idx_instruction_audit_outcome_hourly_range
    ON instruction_audit_outcome_hourly(bucket_at DESC, final_outcome, group_id, client_type);
