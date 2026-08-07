package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	SecurityNotificationSourceInstructionAudit = "instruction_audit"
	SecurityNotificationSourceCyberPolicy      = "cyber_policy"
)

type SecurityNotificationEnqueueInput struct {
	SourceType   string
	SourceID     int64
	UserID       int64
	UserEmail    string
	DedupeScope  string
	UserTemplate string
	OpsTemplate  string
	Variables    map[string]string
	SkipUser     bool
	SkipOps      bool
}

type SecurityNotificationAudienceInput struct {
	SourceType    string
	SourceID      int64
	Audience      string
	UserID        int64
	Recipients    []string
	TemplateEvent string
	Variables     map[string]string
	DedupKey      string
}

type SecurityNotificationOutboxItem struct {
	ID                  int64
	SourceType          string
	SourceID            int64
	Audience            string
	UserID              int64
	Recipients          []string
	SentRecipientHashes []string
	TemplateEvent       string
	Variables           map[string]string
	Attempts            int
	MaxAttempts         int
}

type SecurityNotificationRepository interface {
	Enqueue(ctx context.Context, input SecurityNotificationAudienceInput) error
	ReclaimStale(ctx context.Context) error
	Claim(ctx context.Context) (*SecurityNotificationOutboxItem, error)
	MarkRecipientSent(ctx context.Context, id int64, recipientHash string) error
	MarkSent(ctx context.Context, item SecurityNotificationOutboxItem) error
	MarkFailed(ctx context.Context, item SecurityNotificationOutboxItem, sendErr error, delay time.Duration) error
}

type SecurityNotificationService struct {
	repository   SecurityNotificationRepository
	settingRepo  SettingRepository
	userRepo     UserRepository
	emailService *NotificationEmailService

	lifecycleMu sync.Mutex
	cancel      context.CancelFunc
	wg          sync.WaitGroup
}

func ProvideSecurityNotificationService(
	repository SecurityNotificationRepository,
	settingRepo SettingRepository,
	userRepo UserRepository,
	emailService *NotificationEmailService,
) *SecurityNotificationService {
	service := &SecurityNotificationService{
		repository: repository, settingRepo: settingRepo, userRepo: userRepo, emailService: emailService,
	}
	service.Start(context.Background())
	return service
}

func (s *SecurityNotificationService) Start(ctx context.Context) {
	if s == nil || s.repository == nil {
		return
	}
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.cancel != nil {
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	if err := s.repository.ReclaimStale(runCtx); err != nil {
		slog.Warn("security_notification.outbox_reclaim_failed", "error", err)
	}
	s.wg.Add(1)
	go s.worker(runCtx)
}

func (s *SecurityNotificationService) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.lifecycleMu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *SecurityNotificationService) Enqueue(ctx context.Context, input SecurityNotificationEnqueueInput) error {
	if s == nil || s.repository == nil {
		return errors.New("security notification service unavailable")
	}
	if input.SourceID <= 0 || strings.TrimSpace(input.SourceType) == "" {
		return errors.New("security notification source required")
	}
	userRecipients := normalizeEmails([]string{input.UserEmail})
	opsRecipients := s.opsRecipients(ctx)
	bucket := time.Now().UTC().Truncate(15 * time.Minute).Format(time.RFC3339)
	inputs := []SecurityNotificationAudienceInput{
		{
			SourceType: input.SourceType, SourceID: input.SourceID, Audience: "user",
			UserID: input.UserID, Recipients: userRecipients, TemplateEvent: input.UserTemplate,
			Variables: cloneSecurityNotificationVariables(input.Variables),
			DedupKey:  securityNotificationDedupKey(input.SourceType, "user", input.DedupeScope, bucket),
		},
		{
			SourceType: input.SourceType, SourceID: input.SourceID, Audience: "ops",
			Recipients: opsRecipients, TemplateEvent: input.OpsTemplate,
			Variables: cloneSecurityNotificationVariables(input.Variables),
			DedupKey:  securityNotificationDedupKey(input.SourceType, "ops", input.DedupeScope, bucket),
		},
	}
	var enqueueErrors []error
	for _, item := range inputs {
		if (item.Audience == "user" && input.SkipUser) || (item.Audience == "ops" && input.SkipOps) {
			continue
		}
		if err := s.repository.Enqueue(ctx, item); err != nil {
			enqueueErrors = append(enqueueErrors, fmt.Errorf("%s notification: %w", item.Audience, err))
		}
	}
	return errors.Join(enqueueErrors...)
}

func (s *SecurityNotificationService) opsRecipients(ctx context.Context) []string {
	var config struct {
		Alert struct {
			Recipients []string `json:"recipients"`
		} `json:"alert"`
	}
	if s.settingRepo != nil {
		if raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsEmailNotificationConfig); err == nil {
			_ = json.Unmarshal([]byte(raw), &config)
		}
	}
	if recipients := normalizeEmails(config.Alert.Recipients); len(recipients) > 0 {
		return recipients
	}
	if s.userRepo != nil {
		if admin, err := s.userRepo.GetFirstAdmin(ctx); err == nil && admin != nil {
			return normalizeEmails([]string{admin.Email})
		}
	}
	return []string{}
}

func (s *SecurityNotificationService) worker(ctx context.Context) {
	defer s.wg.Done()
	poll := time.NewTicker(time.Second)
	reclaim := time.NewTicker(time.Minute)
	defer poll.Stop()
	defer reclaim.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-reclaim.C:
			if err := s.repository.ReclaimStale(ctx); err != nil {
				slog.Warn("security_notification.outbox_reclaim_failed", "error", err)
			}
		case <-poll.C:
			s.process(ctx)
		}
	}
}

func (s *SecurityNotificationService) process(ctx context.Context) {
	for processed := 0; processed < 20; processed++ {
		item, err := s.repository.Claim(ctx)
		if errors.Is(err, sql.ErrNoRows) {
			return
		}
		if err != nil {
			slog.Warn("security_notification.outbox_claim_failed", "error", err)
			return
		}
		sendErr := s.send(ctx, item)
		if sendErr == nil {
			if err := s.repository.MarkSent(ctx, *item); err != nil {
				slog.Warn("security_notification.outbox_mark_sent_failed", "outbox_id", item.ID, "error", err)
			}
			continue
		}
		if err := s.repository.MarkFailed(ctx, *item, sendErr, securityNotificationRetryDelay(item.Attempts)); err != nil {
			slog.Warn("security_notification.outbox_mark_failed_failed", "outbox_id", item.ID, "error", err)
		}
	}
}

func (s *SecurityNotificationService) send(ctx context.Context, item *SecurityNotificationOutboxItem) error {
	if s.emailService == nil {
		return errors.New("notification email service unavailable")
	}
	if item == nil {
		return errors.New("security notification item required")
	}
	var sendErrors []error
	for _, recipient := range item.Recipients {
		hash := notificationEmailHash(recipient)
		if hash == "" || stringSliceContains(item.SentRecipientHashes, hash) {
			continue
		}
		sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := s.emailService.Send(sendCtx, NotificationEmailSendInput{
			Event: item.TemplateEvent, RecipientEmail: recipient,
			RecipientName: emailRecipientName(recipient), UserID: item.UserID,
			SourceType: item.SourceType + ":" + item.Audience,
			SourceID:   strconv.FormatInt(item.SourceID, 10), Variables: item.Variables,
		})
		cancel()
		if err != nil {
			sendErrors = append(sendErrors, fmt.Errorf("recipient %s: %w", hash, err))
			continue
		}
		if err := s.repository.MarkRecipientSent(ctx, item.ID, hash); err != nil {
			return err
		}
		item.SentRecipientHashes = append(item.SentRecipientHashes, hash)
	}
	return errors.Join(sendErrors...)
}

func securityNotificationRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := 5 * time.Second * time.Duration(1<<min(attempt-1, 8))
	if delay > 15*time.Minute {
		return 15 * time.Minute
	}
	return delay
}

func securityNotificationDedupKey(sourceType, audience, scope, bucket string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{sourceType, audience, scope, bucket}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func cloneSecurityNotificationVariables(input map[string]string) map[string]string {
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func stringSliceContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
