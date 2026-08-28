ALTER TABLE IF EXISTS batch_image_jobs
    ADD COLUMN IF NOT EXISTS group_id BIGINT;

COMMENT ON COLUMN batch_image_jobs.group_id IS
    'Submission-time group snapshot for usage attribution and account statistics pricing';
