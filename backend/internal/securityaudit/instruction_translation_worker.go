package securityaudit

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
)

const instructionTranslationMaxWorkers = 16

func (s *InstructionService) translationWorker(ctx context.Context, workerID int) {
	defer s.wg.Done()
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snapshot := s.snapshot.Load()
			if snapshot == nil || !snapshot.Runtime.TranslationEnabled ||
				workerID >= snapshot.Runtime.TranslationMaxConcurrency {
				continue
			}
			for {
				job, claimed, err := s.repository.ClaimTranslationJob(ctx, time.Now().UTC())
				if err != nil {
					slog.Warn("instruction_audit.translation_claim_failed", "error", err)
					break
				}
				if !claimed {
					break
				}
				s.translationActive.Add(1)
				s.processInstructionTranslationSafely(ctx, job, snapshot.Runtime)
				s.translationActive.Add(-1)
			}
		}
	}
}

func (s *InstructionService) processInstructionTranslationSafely(
	ctx context.Context,
	job *InstructionTranslationJob,
	runtime InstructionRuntimeConfig,
) {
	defer func() {
		if recover() != nil {
			s.finishInstructionTranslationFailure(ctx, job, 0, nil, nil, errors.New("worker_panic"))
		}
	}()
	s.processInstructionTranslation(ctx, job, runtime)
}

func (s *InstructionService) translationMaintenanceLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().UTC()
			if _, err := s.repository.ExpireTranslationJobs(ctx, now); err != nil {
				slog.Warn("instruction_audit.translation_expire_failed", "error", err)
			}
			if _, err := s.repository.ReclaimTranslationJobs(ctx, now.Add(-2*time.Minute)); err != nil {
				slog.Warn("instruction_audit.translation_reclaim_failed", "error", err)
			}
		}
	}
}

func (s *InstructionService) processInstructionTranslation(
	ctx context.Context,
	job *InstructionTranslationJob,
	runtime InstructionRuntimeConfig,
) {
	if job == nil {
		return
	}
	source, err := s.loadInstructionTranslationSource(ctx, job)
	if err != nil {
		s.finishInstructionTranslationFailure(ctx, job, 0, nil, nil, err)
		return
	}
	if source.Bytes > runtime.TranslationMaxBytes {
		s.finishInstructionTranslationFailure(ctx, job, 0, nil, nil, errors.New("source_too_large"))
		return
	}
	redacted, redactions := redactInstructionTranslationText(source.Plaintext)
	chunks, err := splitInstructionTranslationUTF8(redacted, runtime.TranslationChunkBytes)
	if err != nil {
		s.finishInstructionTranslationFailure(ctx, job, 0, redactions, nil, err)
		return
	}
	if err = s.repository.SetTranslationJobProgress(ctx, job.ID, job.ClaimVersion, len(chunks), 0); err != nil {
		s.translationFailed.Add(1)
		return
	}
	translatedChunks := make([]string, 0, len(chunks))
	providerLatency := time.Duration(0)
	translatedBytes := 0
	for index, chunk := range chunks {
		if !time.Now().UTC().Before(job.ExpiresAt) {
			s.finishInstructionTranslationFailure(
				ctx, job, int(providerLatency.Milliseconds()), redactions, translatedChunks, errors.New("result_expired"),
			)
			return
		}
		if s.translator == nil {
			s.finishInstructionTranslationFailure(
				ctx, job, int(providerLatency.Milliseconds()), redactions, translatedChunks, errInstructionTranslationUnavailable,
			)
			return
		}
		callCtx, cancel := context.WithTimeout(ctx, time.Duration(runtime.TranslationTimeoutMS)*time.Millisecond)
		startedAt := time.Now()
		translated, translateErr := s.translator.Translate(
			callCtx, runtime, job.Provider, job.TargetLanguage, chunk,
		)
		providerLatency += time.Since(startedAt)
		cancel()
		if translateErr != nil {
			s.finishInstructionTranslationFailure(
				ctx, job, int(providerLatency.Milliseconds()), redactions, translatedChunks, translateErr,
			)
			return
		}
		translatedBytes += len([]byte(translated))
		if translatedBytes > runtime.TranslationMaxBytes*4 {
			s.finishInstructionTranslationFailure(
				ctx, job, int(providerLatency.Milliseconds()), redactions, translatedChunks, errors.New("result_too_large"),
			)
			return
		}
		translatedChunks = append(translatedChunks, translated)
		if err = s.repository.SetTranslationJobProgress(
			ctx, job.ID, job.ClaimVersion, len(chunks), index+1,
		); err != nil {
			s.translationFailed.Add(1)
			return
		}
	}
	translated := restoreInstructionTranslationText(strings.Join(translatedChunks, ""), redactions)
	if err = s.storeInstructionTranslationResult(ctx, job, translated); err != nil {
		s.finishInstructionTranslationFailure(
			ctx, job, int(providerLatency.Milliseconds()), redactions, nil, err,
		)
		return
	}
	if err = s.repository.CompleteTranslationJob(
		ctx, job.ID, job.ClaimVersion, "succeeded", len(chunks), len([]byte(translated)),
		len(redactions), int(providerLatency.Milliseconds()), "",
	); err != nil {
		s.translationFailed.Add(1)
		return
	}
	s.translationProcessed.Add(1)
}

func (s *InstructionService) finishInstructionTranslationFailure(
	ctx context.Context,
	job *InstructionTranslationJob,
	providerLatencyMS int,
	redactions map[string]string,
	translatedChunks []string,
	err error,
) {
	if job == nil {
		return
	}
	code := instructionTranslationErrorCode(err)
	if len(translatedChunks) == 0 && instructionTranslationRetryable(err) && job.Attempts < job.MaxAttempts {
		delay := time.Duration(1<<min(job.Attempts, 6)) * time.Second
		if retryErr := s.repository.RetryTranslationJob(
			ctx, job.ID, job.ClaimVersion, time.Now().UTC().Add(delay), code,
		); retryErr == nil {
			return
		}
	}
	status := "failed"
	completedChunks, resultBytes := 0, 0
	if len(translatedChunks) > 0 {
		status = "partial"
		completedChunks = len(translatedChunks)
		partial := restoreInstructionTranslationText(strings.Join(translatedChunks, ""), redactions)
		if storeErr := s.storeInstructionTranslationResult(ctx, job, partial); storeErr == nil {
			resultBytes = len([]byte(partial))
		} else {
			status, completedChunks = "failed", 0
			code = instructionTranslationErrorCode(storeErr)
		}
	}
	if updateErr := s.repository.CompleteTranslationJob(
		ctx, job.ID, job.ClaimVersion, status, completedChunks, resultBytes,
		len(redactions), providerLatencyMS, code,
	); updateErr != nil {
		slog.Warn("instruction_audit.translation_finish_failed", "job_id", job.ID, "error", updateErr)
	}
	s.translationFailed.Add(1)
}

func (s *InstructionService) loadInstructionTranslationSource(
	ctx context.Context,
	job *InstructionTranslationJob,
) (*instructionTranslationSource, error) {
	if s.evidenceCipher == nil || !s.evidenceCipher.Available() {
		return nil, errInstructionEvidenceEncryptionUnavailable
	}
	if job.ResourceType == "event" {
		event, err := s.repository.GetEvent(ctx, job.ResourceID)
		if err != nil {
			return nil, err
		}
		if event.EvidenceStatus != "stored" || (event.EvidenceExpiresAt != nil && !time.Now().UTC().Before(*event.EvidenceExpiresAt)) {
			return nil, errors.New("source_expired")
		}
		items, err := s.repository.ListEvidence(ctx, event.ID)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.Source != job.FieldName {
				continue
			}
			plaintext, decryptErr := s.evidenceCipher.Decrypt(item.Source, item.Digest, item.Ciphertext)
			if decryptErr != nil || sha256Hex(plaintext) != item.Digest || len([]byte(plaintext)) != item.PlaintextBytes {
				return nil, errors.New("source_integrity_failed")
			}
			return &instructionTranslationSource{Plaintext: plaintext, Digest: item.Digest, Bytes: item.PlaintextBytes}, nil
		}
		return nil, errors.New("source_unavailable")
	}
	hash, err := s.repository.GetHash(ctx, job.ResourceID)
	if err != nil {
		return nil, err
	}
	raw, err := s.repository.GetHashRaw(ctx, hash.ID)
	if err != nil {
		return nil, err
	}
	if raw.Status != "stored" || (raw.ExpiresAt != nil && !time.Now().UTC().Before(*raw.ExpiresAt)) {
		return nil, errors.New("source_expired")
	}
	plaintext, err := s.evidenceCipher.DecryptHashRaw(hash.Digest, raw.Ciphertext)
	if err != nil || sha256Hex(plaintext) != hash.Digest || len([]byte(plaintext)) != raw.ContentBytes {
		return nil, errors.New("source_integrity_failed")
	}
	return &instructionTranslationSource{Plaintext: plaintext, Digest: hash.Digest, Bytes: raw.ContentBytes}, nil
}

func (s *InstructionService) storeInstructionTranslationResult(
	ctx context.Context,
	job *InstructionTranslationJob,
	translated string,
) error {
	if s.redis == nil || s.secretEncryptor == nil || job == nil || translated == "" {
		return errInstructionTranslationUnavailable
	}
	ttl := time.Until(job.ExpiresAt)
	if ttl <= 0 {
		return errors.New("result_expired")
	}
	ciphertext, err := s.secretEncryptor.Encrypt(translated)
	if err != nil {
		return errInstructionTranslationUnavailable
	}
	return s.redis.Set(ctx, instructionTranslationResultKey(job.ID), ciphertext, ttl).Err()
}

func instructionTranslationErrorCode(err error) string {
	if err == nil {
		return ""
	}
	var providerErr *instructionTranslationProviderError
	if errors.As(err, &providerErr) && providerErr.code != "" {
		return providerErr.code
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return "provider_timeout"
	case errors.Is(err, errInstructionEvidenceEncryptionUnavailable):
		return "encryption_unavailable"
	case errors.Is(err, errInstructionTranslationUnavailable):
		return "result_store_unavailable"
	}
	message := err.Error()
	for _, code := range []string{
		"source_too_large", "source_expired", "source_integrity_failed",
		"source_unavailable", "result_expired", "result_too_large", "worker_panic",
	} {
		if message == code {
			return code
		}
	}
	return "translation_failed"
}

func instructionTranslationRetryable(err error) bool {
	var providerErr *instructionTranslationProviderError
	return errors.As(err, &providerErr) && providerErr.retryable
}
