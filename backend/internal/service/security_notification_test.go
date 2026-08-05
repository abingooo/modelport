package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type securityNotificationRepositoryStub struct {
	inputs     []SecurityNotificationAudienceInput
	claimed    *SecurityNotificationOutboxItem
	claimCount int
	markFailed []time.Duration
	markSent   []SecurityNotificationOutboxItem
	markErr    error
	failedErr  error
}

func (r *securityNotificationRepositoryStub) Enqueue(_ context.Context, input SecurityNotificationAudienceInput) error {
	r.inputs = append(r.inputs, input)
	return r.failedErr
}

func (r *securityNotificationRepositoryStub) ReclaimStale(context.Context) error { return nil }

func (r *securityNotificationRepositoryStub) Claim(context.Context) (*SecurityNotificationOutboxItem, error) {
	if r.claimCount > 0 {
		r.claimCount--
		return r.claimed, nil
	}
	return nil, sql.ErrNoRows
}

func (r *securityNotificationRepositoryStub) MarkRecipientSent(context.Context, int64, string) error {
	return r.markErr
}

func (r *securityNotificationRepositoryStub) MarkSent(_ context.Context, item SecurityNotificationOutboxItem) error {
	r.markSent = append(r.markSent, item)
	return r.markErr
}

func (r *securityNotificationRepositoryStub) MarkFailed(_ context.Context, _ SecurityNotificationOutboxItem, _ error, delay time.Duration) error {
	r.markFailed = append(r.markFailed, delay)
	return r.markErr
}

type securityNotificationSettingRepositoryStub struct {
	SettingRepository
	value string
	err   error
}

func (r *securityNotificationSettingRepositoryStub) GetValue(context.Context, string) (string, error) {
	return r.value, r.err
}

type securityNotificationUserRepositoryStub struct {
	UserRepository
	admin *User
	err   error
}

func (r *securityNotificationUserRepositoryStub) GetFirstAdmin(context.Context) (*User, error) {
	return r.admin, r.err
}

func TestSecurityNotificationEnqueueSplitsUserAndOperationsRecipients(t *testing.T) {
	repository := &securityNotificationRepositoryStub{}
	settingRepository := &securityNotificationSettingRepositoryStub{value: `{"alert":{"recipients":[" Ops@Example.com ","ops@example.com","second@example.com"]}}`}
	notifications := &SecurityNotificationService{repository: repository, settingRepo: settingRepository}
	variables := map[string]string{"model": "gpt-test"}

	require.NoError(t, notifications.Enqueue(context.Background(), SecurityNotificationEnqueueInput{
		SourceType: SecurityNotificationSourceInstructionAudit, SourceID: 7, UserID: 42,
		UserEmail: " User@Example.com ", DedupeScope: "42:3",
		UserTemplate: NotificationEmailEventInstructionAuditUserNotice,
		OpsTemplate:  NotificationEmailEventInstructionAuditOpsNotice, Variables: variables,
	}))

	require.Len(t, repository.inputs, 2)
	require.Equal(t, "user", repository.inputs[0].Audience)
	require.Equal(t, []string{"user@example.com"}, repository.inputs[0].Recipients)
	require.Equal(t, "ops", repository.inputs[1].Audience)
	require.Equal(t, []string{"ops@example.com", "second@example.com"}, repository.inputs[1].Recipients)
	require.NotEqual(t, repository.inputs[0].DedupKey, repository.inputs[1].DedupKey)
	variables["model"] = "mutated"
	require.Equal(t, "gpt-test", repository.inputs[0].Variables["model"])
}

func TestSecurityNotificationOpsRecipientsFallBackToFirstAdmin(t *testing.T) {
	repository := &securityNotificationRepositoryStub{}
	users := &securityNotificationUserRepositoryStub{admin: &User{Email: " Admin@Example.com "}}
	notifications := &SecurityNotificationService{
		repository:  repository,
		settingRepo: &securityNotificationSettingRepositoryStub{err: errors.New("setting unavailable")},
		userRepo:    users,
	}

	require.NoError(t, notifications.Enqueue(context.Background(), SecurityNotificationEnqueueInput{
		SourceType: SecurityNotificationSourceCyberPolicy, SourceID: 9,
		UserTemplate: NotificationEmailEventCyberPolicyNotice,
		OpsTemplate:  NotificationEmailEventCyberPolicyOpsNotice,
	}))
	require.Len(t, repository.inputs, 2)
	require.Empty(t, repository.inputs[0].Recipients)
	require.Equal(t, []string{"admin@example.com"}, repository.inputs[1].Recipients)
}

func TestSecurityNotificationProcessMarksEmailFailureForRetry(t *testing.T) {
	repository := &securityNotificationRepositoryStub{
		claimed: &SecurityNotificationOutboxItem{
			ID: 11, Attempts: 8, MaxAttempts: 8, Recipients: []string{"ops@example.com"},
		},
		claimCount: 1,
	}
	notifications := &SecurityNotificationService{repository: repository}
	notifications.process(context.Background())
	require.Len(t, repository.markFailed, 1)
	require.Equal(t, 5*time.Second*128, repository.markFailed[0])
	require.Empty(t, repository.markSent)
}

func TestSecurityNotificationRetryDelayIsBounded(t *testing.T) {
	require.Equal(t, 5*time.Second, securityNotificationRetryDelay(1))
	require.Equal(t, 15*time.Minute, securityNotificationRetryDelay(100))
}
