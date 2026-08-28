-- Preserve the legacy ModelPort behavior for databases that already contain
-- referral history. Fresh installations keep default inviter binding disabled.

INSERT INTO settings(key, value, updated_at)
SELECT
    'affiliate_reward_program_config',
    jsonb_build_object(
        'version', 1,
        'enabled', true,
        'legacy_approval_cutoff', '2026-07-05T22:00:00Z',
        'registration', jsonb_build_object(
            'enabled', true,
            'default_inviter_enabled', true,
            'default_inviter_user_id', 1,
            'inviter_bonus', 1,
            'invitee_trial_amount', 3,
            'invitee_trial_group_id', 50,
            'invitee_trial_days', 3
        ),
        'first_recharge', jsonb_build_object(
            'enabled', true,
            'inviter_bonus', 2,
            'invitee_bonus_percent', 10
        )
    )::text,
    NOW()
WHERE EXISTS (SELECT 1 FROM referral.reward_reviews LIMIT 1)
ON CONFLICT (key) DO UPDATE
SET value = (
        settings.value::jsonb ||
        jsonb_build_object(
            'registration',
            COALESCE(settings.value::jsonb->'registration', '{}'::jsonb) ||
            CASE
                WHEN COALESCE(settings.value::jsonb->'registration', '{}'::jsonb) ? 'default_inviter_enabled'
                    THEN '{}'::jsonb
                ELSE jsonb_build_object('default_inviter_enabled', true)
            END ||
            CASE
                WHEN COALESCE(settings.value::jsonb->'registration', '{}'::jsonb) ? 'default_inviter_user_id'
                    THEN '{}'::jsonb
                ELSE jsonb_build_object('default_inviter_user_id', 1)
            END
        )
    )::text,
    updated_at = NOW()
WHERE NOT (COALESCE(settings.value::jsonb->'registration', '{}'::jsonb) ? 'default_inviter_enabled')
   OR NOT (COALESCE(settings.value::jsonb->'registration', '{}'::jsonb) ? 'default_inviter_user_id');
