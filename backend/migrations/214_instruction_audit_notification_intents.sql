ALTER TABLE security_notification_outbox
    DROP CONSTRAINT IF EXISTS chk_security_notification_status;
ALTER TABLE security_notification_outbox
    ADD CONSTRAINT chk_security_notification_status CHECK (
        status IN (
            'pending', 'processing', 'retry', 'sent', 'failed',
            'suppressed', 'no_recipient', 'enqueue_failed'
        )
    );
