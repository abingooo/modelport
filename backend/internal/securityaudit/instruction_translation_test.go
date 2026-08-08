package securityaudit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	modelportrepository "github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type instructionTranslatorFunc func(context.Context, InstructionRuntimeConfig, string, string, string) (string, error)

func (f instructionTranslatorFunc) Translate(
	ctx context.Context,
	config InstructionRuntimeConfig,
	provider string,
	targetLanguage string,
	content string,
) (string, error) {
	return f(ctx, config, provider, targetLanguage, content)
}

func TestInstructionTranslationRedactionAndUTF8ChunkingAreReversible(t *testing.T) {
	original := "账号 user@example.test\nAuthorization: Bearer abcdefghijklmnop\nkey=sk-1234567890abcdef\n" + strings.Repeat("港口模型", 400)
	redacted, replacements := redactInstructionTranslationText(original)
	require.Len(t, replacements, 3)
	require.NotContains(t, redacted, "user@example.test")
	require.NotContains(t, redacted, "abcdefghijklmnop")
	require.NotContains(t, redacted, "sk-1234567890abcdef")
	chunks, err := splitInstructionTranslationUTF8(redacted, 257)
	require.NoError(t, err)
	require.Greater(t, len(chunks), 2)
	for _, chunk := range chunks {
		require.True(t, utf8.ValidString(chunk))
		require.LessOrEqual(t, len([]byte(chunk)), 257)
	}
	require.Equal(t, redacted, strings.Join(chunks, ""))
	require.Equal(t, original, restoreInstructionTranslationText(strings.Join(chunks, ""), replacements))
	_, err = splitInstructionTranslationUTF8(string([]byte{0xff, 0xfe}), 256)
	require.Error(t, err)
}

func TestInstructionTranslationChunkingKeepsRedactionPlaceholdersAtomic(t *testing.T) {
	original := "prefix-1234 Authorization: Bearer abcdefghijklmnop suffix"
	redacted, replacements := redactInstructionTranslationText(original)
	require.Len(t, replacements, 1)

	chunks, err := splitInstructionTranslationUTF8(redacted, 24)
	require.NoError(t, err)
	require.Equal(t, redacted, strings.Join(chunks, ""))
	for placeholder := range replacements {
		containingChunks := 0
		for _, chunk := range chunks {
			if strings.Contains(chunk, placeholder) {
				containingChunks++
			}
		}
		require.Equal(t, 1, containingChunks)
	}
	require.Equal(t, original, restoreInstructionTranslationText(strings.Join(chunks, ""), replacements))
}

func TestInstructionTranslationRedactionAvoidsExistingPlaceholderCollisions(t *testing.T) {
	original := "literal [[MP_REDACTED_0001]] Authorization: Bearer abcdefghijklmnop"
	redacted, replacements := redactInstructionTranslationText(original)
	require.Len(t, replacements, 1)
	require.Contains(t, redacted, "literal [[MP_REDACTED_0001]]")
	require.Contains(t, redacted, "[[MP_REDACTED_0002]]")
	require.NotContains(t, redacted, "abcdefghijklmnop")
	require.Equal(t, original, restoreInstructionTranslationText(redacted, replacements))
}

func TestOpenAIInstructionTranslatorUsesDedicatedStructuredRequest(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		require.Equal(t, "/v1/chat/completions", request.URL.Path)
		require.Equal(t, "Bearer translation-token", request.Header.Get("Authorization"))
		require.Equal(t, instructionTranslationPurposeHeader, request.Header.Get("X-ModelPort-Internal-Purpose"))
		require.NoError(t, json.NewDecoder(request.Body).Decode(&captured))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"translation\":\"翻译结果\"}"}}]}`))
	}))
	t.Cleanup(server.Close)

	translated, err := NewOpenAIInstructionTranslator().Translate(context.Background(), InstructionRuntimeConfig{
		ExternalTranslationEnabled: true, TranslationBaseURL: server.URL,
		TranslationModel: "translation-model", TranslationToken: "translation-token",
		TranslationTimeoutMS: 1000,
	}, "external", "zh-CN", "[[MP_REDACTED_0001]] source")
	require.NoError(t, err)
	require.Equal(t, "翻译结果", translated)
	require.Equal(t, "translation-model", captured["model"])
	require.Equal(t, "json_schema", captured["response_format"].(map[string]any)["type"])
	messages := captured["messages"].([]any)
	require.Len(t, messages, 2)
	userMessage := messages[1].(map[string]any)["content"].(string)
	require.Contains(t, userMessage, "[[MP_REDACTED_0001]]")
	require.NotContains(t, userMessage, "translation-token")
}

func newInstructionTranslationIntegrationService(
	t *testing.T,
	translator InstructionTranslator,
) (*InstructionService, *InstructionRepository, *redis.Client, int64) {
	t.Helper()
	db := openInstructionAuditIntegrationDB(t)
	repository := NewInstructionRepository(db)
	adminID := insertInstructionAuditUser(t, db, "translation-admin@example.test", "admin")
	insertInstructionSensitiveTestGrant(t, db, adminID, "emergency_cli")
	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { require.NoError(t, redisClient.Close()) })
	cfg := &config.Config{
		Totp: config.TotpConfig{
			EncryptionKey: strings.Repeat("45", 32), EncryptionKeyConfigured: true,
		},
	}
	cipher, err := NewInstructionEvidenceCipher(cfg)
	require.NoError(t, err)
	encryptor, err := modelportrepository.NewAESEncryptor(cfg)
	require.NoError(t, err)
	translationToken, err := encryptor.Encrypt("translation-token")
	require.NoError(t, err)
	_, err = repository.db.Exec(`
		UPDATE instruction_audit_runtime_config
		SET translation_enabled = TRUE, external_translation_enabled = TRUE,
			translation_base_url = 'https://translation.example.test',
			translation_model = 'translation-model', translation_token_ciphertext = $1,
			translation_timeout_ms = 1000, translation_max_concurrency = 2,
			translation_chunk_bytes = 1024, translation_max_bytes = 1048576,
			translation_result_ttl_seconds = 1800
		WHERE id = 1`, translationToken)
	require.NoError(t, err)
	service := NewInstructionService(repository, redisClient, nil)
	service.evidenceCipher = cipher
	service.secretEncryptor = encryptor
	service.translator = translator
	snapshot, err := repository.LoadSnapshot(context.Background())
	require.NoError(t, err)
	snapshot.Runtime.TranslationEnabled = true
	snapshot.Runtime.ExternalTranslationEnabled = true
	snapshot.Runtime.TranslationBaseURL = "https://translation.example.test"
	snapshot.Runtime.TranslationModel = "translation-model"
	snapshot.Runtime.TranslationToken = "translation-token"
	snapshot.Runtime.TranslationTimeoutMS = 1000
	snapshot.Runtime.TranslationMaxConcurrency = 2
	snapshot.Runtime.TranslationChunkBytes = 1024
	snapshot.Runtime.TranslationMaxBytes = 1 << 20
	snapshot.Runtime.TranslationResultTTLSeconds = 1800
	service.snapshot.Store(snapshot)
	return service, repository, redisClient, adminID
}

func TestInstructionTranslationJobStoresOnlyEncryptedTTLResult(t *testing.T) {
	received := make([]string, 0)
	translator := instructionTranslatorFunc(func(_ context.Context, _ InstructionRuntimeConfig, provider, language, content string) (string, error) {
		require.Equal(t, "external", provider)
		require.Equal(t, "zh-CN", language)
		require.NotContains(t, content, "translation-user@example.test")
		require.NotContains(t, content, "sk-translation-secret")
		received = append(received, content)
		return content, nil
	})
	service, repository, redisClient, adminID := newInstructionTranslationIntegrationService(t, translator)
	ctx := instructionSensitiveTestContext(t, repository.db, adminID)
	original := "translation-user@example.test sk-translation-secret\n" + strings.Repeat("模型港口", 500)
	hash, err := service.CreateHash(ctx, CreateInstructionHashRequest{
		RawContent: original, Name: "translation source", ObservedSource: "instructions", Status: "active",
	}, adminID)
	require.NoError(t, err)
	job, err := service.CreateTranslationJob(ctx, InstructionTranslationRequest{
		ResourceType: "hash", ResourceID: hash.ID, FieldName: "instructions",
		TargetLanguage: "zh-CN", Provider: "external",
	}, adminID, InstructionSensitiveAccess{ActorID: adminID, RequestID: "translation-create"})
	require.NoError(t, err)
	claimed, ok, err := repository.ClaimTranslationJob(ctx, job.CreatedAt.Add(time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	service.processInstructionTranslation(ctx, claimed, service.snapshot.Load().Runtime)
	require.Greater(t, len(received), 1)

	completed, err := service.GetTranslationJob(ctx, job.ID, InstructionSensitiveAccess{
		ActorID: adminID, RequestID: "translation-reveal",
	})
	require.NoError(t, err)
	require.Equal(t, "succeeded", completed.Status)
	require.Equal(t, original, completed.TranslatedText)
	require.Equal(t, len(received), completed.ChunkCount)
	require.Equal(t, completed.ChunkCount, completed.CompletedChunks)
	require.Equal(t, 2, completed.RedactionCount)

	stored, err := redisClient.Get(ctx, instructionTranslationResultKey(job.ID)).Result()
	require.NoError(t, err)
	require.NotContains(t, stored, original)
	require.NotContains(t, stored, "sk-translation-secret")
	var databaseRow string
	require.NoError(t, repository.db.QueryRow(`
		SELECT row_to_json(j)::TEXT FROM instruction_audit_translation_jobs j WHERE id = $1`, job.ID).Scan(&databaseRow))
	require.NotContains(t, databaseRow, original)
	require.NotContains(t, databaseRow, "sk-translation-secret")
	var translateAccesses, revealAccesses int64
	require.NoError(t, repository.db.QueryRow(`
		SELECT
			COUNT(*) FILTER (WHERE action = 'translate' AND succeeded),
			COUNT(*) FILTER (WHERE action = 'reveal' AND succeeded)
		FROM instruction_audit_sensitive_access_logs
		WHERE resource_type = 'translation' AND resource_id = $1`, job.ID).Scan(
		&translateAccesses, &revealAccesses,
	))
	require.EqualValues(t, 1, translateAccesses)
	require.EqualValues(t, 1, revealAccesses)

	require.NoError(t, redisClient.Del(ctx, instructionTranslationResultKey(job.ID)).Err())
	expired, err := service.GetTranslationJob(ctx, job.ID, InstructionSensitiveAccess{ActorID: adminID})
	require.NoError(t, err)
	require.Equal(t, "expired", expired.Status)
}

func TestInstructionTranslationJobPreservesPartialResult(t *testing.T) {
	call := 0
	translator := instructionTranslatorFunc(func(_ context.Context, _ InstructionRuntimeConfig, _, _, content string) (string, error) {
		call++
		if call == 2 {
			return "", &instructionTranslationProviderError{code: "invalid_response"}
		}
		return content, nil
	})
	service, repository, _, adminID := newInstructionTranslationIntegrationService(t, translator)
	ctx := instructionSensitiveTestContext(t, repository.db, adminID)
	original := "Bearer partial-secret-token\n" + strings.Repeat("港", 900)
	hash, err := service.CreateHash(ctx, CreateInstructionHashRequest{
		RawContent: original, Name: "partial source", ObservedSource: "input1", Status: "active",
	}, adminID)
	require.NoError(t, err)
	job, err := service.CreateTranslationJob(ctx, InstructionTranslationRequest{
		ResourceType: "hash", ResourceID: hash.ID, FieldName: "input1",
		TargetLanguage: "zh-CN", Provider: "external",
	}, adminID, InstructionSensitiveAccess{ActorID: adminID})
	require.NoError(t, err)
	claimed, ok, err := repository.ClaimTranslationJob(ctx, job.CreatedAt.Add(time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	service.processInstructionTranslation(ctx, claimed, service.snapshot.Load().Runtime)
	partial, err := service.GetTranslationJob(ctx, job.ID, InstructionSensitiveAccess{ActorID: adminID})
	require.NoError(t, err)
	require.Equal(t, "partial", partial.Status)
	require.Equal(t, 1, partial.CompletedChunks)
	require.Equal(t, "invalid_response", partial.ErrorCode)
	require.NotEmpty(t, partial.TranslatedText)
	require.Contains(t, partial.TranslatedText, "Bearer partial-secret-token")
	require.Less(t, len(partial.TranslatedText), len(original))
}

func TestInstructionTranslationJobRetriesTransientFailureThenSucceeds(t *testing.T) {
	calls := 0
	translator := instructionTranslatorFunc(func(_ context.Context, _ InstructionRuntimeConfig, _, _, content string) (string, error) {
		calls++
		if calls == 1 {
			return "", &instructionTranslationProviderError{code: "provider_unavailable", retryable: true}
		}
		return "translated:" + content, nil
	})
	service, repository, _, adminID := newInstructionTranslationIntegrationService(t, translator)
	ctx := instructionSensitiveTestContext(t, repository.db, adminID)
	hash, err := service.CreateHash(ctx, CreateInstructionHashRequest{
		RawContent: "retry translation", Name: "retry source", ObservedSource: "instructions", Status: "active",
	}, adminID)
	require.NoError(t, err)
	job, err := service.CreateTranslationJob(ctx, InstructionTranslationRequest{
		ResourceType: "hash", ResourceID: hash.ID, FieldName: "instructions",
		TargetLanguage: "zh-CN", Provider: "external",
	}, adminID, InstructionSensitiveAccess{ActorID: adminID})
	require.NoError(t, err)

	first, ok, err := repository.ClaimTranslationJob(ctx, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, ok)
	service.processInstructionTranslation(ctx, first, service.snapshot.Load().Runtime)
	retrying, err := repository.GetTranslationJob(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, "retry", retrying.Status)
	require.Equal(t, 1, retrying.Attempts)
	require.Equal(t, "provider_unavailable", retrying.ErrorCode)

	second, ok, err := repository.ClaimTranslationJob(ctx, time.Now().UTC().Add(5*time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	service.processInstructionTranslation(ctx, second, service.snapshot.Load().Runtime)
	completed, err := service.GetTranslationJob(ctx, job.ID, InstructionSensitiveAccess{ActorID: adminID})
	require.NoError(t, err)
	require.Equal(t, "succeeded", completed.Status)
	require.Equal(t, 2, completed.Attempts)
	require.Equal(t, "translated:retry translation", completed.TranslatedText)
	require.Equal(t, 2, calls)
}

func TestInstructionTranslationJobSupportsEventEvidence(t *testing.T) {
	translator := instructionTranslatorFunc(func(_ context.Context, _ InstructionRuntimeConfig, _, _, content string) (string, error) {
		return "translated:" + content, nil
	})
	service, repository, _, adminID := newInstructionTranslationIntegrationService(t, translator)
	ctx := instructionSensitiveTestContext(t, repository.db, adminID)
	original := "event evidence source"
	decision := &InstructionDecision{
		Allow: false, InitialReason: "hash_mismatch", FinalReason: "hash_mismatch",
		FinalOutcome: InstructionOutcomeBlocked, PolicyAction: InstructionPolicyActionBlock,
		Instructions: InstructionFieldResult{Result: "missing"},
		Input1: InstructionFieldResult{
			Present: true, SHA256: sha256Hex(original), Result: "mismatch", Plaintext: original,
		},
		ConfigVersion: 1,
	}
	evidenceStatus, evidenceExpiresAt, evidence := service.prepareEvidence(ctx, decision)
	require.Equal(t, "stored", evidenceStatus)
	eventID, err := repository.RecordEvent(ctx, Request{
		RequestID: "translation-event", UserID: adminID, UserEmail: "translation-admin@example.test",
		InstructionClientType: InstructionClientCodexCLI, Model: "gpt-test",
		Endpoint: "/v1/responses", Stage: "http",
	}, decision, evidenceStatus, evidenceExpiresAt, evidence)
	require.NoError(t, err)
	job, err := service.CreateTranslationJob(ctx, InstructionTranslationRequest{
		ResourceType: "event", ResourceID: eventID, FieldName: "input1",
		TargetLanguage: "zh-CN", Provider: "external",
	}, adminID, InstructionSensitiveAccess{ActorID: adminID})
	require.NoError(t, err)
	claimed, ok, err := repository.ClaimTranslationJob(ctx, job.CreatedAt.Add(time.Second))
	require.NoError(t, err)
	require.True(t, ok)
	service.processInstructionTranslation(ctx, claimed, service.snapshot.Load().Runtime)
	completed, err := service.GetTranslationJob(ctx, job.ID, InstructionSensitiveAccess{ActorID: adminID})
	require.NoError(t, err)
	require.Equal(t, "succeeded", completed.Status)
	require.Equal(t, "translated:"+original, completed.TranslatedText)
}

func TestInstructionTranslationBackgroundWorkerCompletesQueuedJob(t *testing.T) {
	translator := instructionTranslatorFunc(func(_ context.Context, _ InstructionRuntimeConfig, _, _, content string) (string, error) {
		return content, nil
	})
	service, repository, _, adminID := newInstructionTranslationIntegrationService(t, translator)
	ctx := instructionSensitiveTestContext(t, repository.db, adminID)
	hash, err := service.CreateHash(ctx, CreateInstructionHashRequest{
		RawContent: "background translation", Name: "background source",
		ObservedSource: "instructions", Status: "active",
	}, adminID)
	require.NoError(t, err)
	job, err := service.CreateTranslationJob(ctx, InstructionTranslationRequest{
		ResourceType: "hash", ResourceID: hash.ID, FieldName: "instructions",
		TargetLanguage: "zh-CN", Provider: "external",
	}, adminID, InstructionSensitiveAccess{ActorID: adminID})
	require.NoError(t, err)
	runCtx, cancel := context.WithCancel(context.Background())
	require.NoError(t, service.Start(runCtx))
	t.Cleanup(func() {
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer shutdownCancel()
		require.NoError(t, service.Shutdown(shutdownCtx))
	})
	require.Eventually(t, func() bool {
		current, getErr := service.GetTranslationJob(ctx, job.ID, InstructionSensitiveAccess{ActorID: adminID})
		return getErr == nil && current.Status == "succeeded" && current.TranslatedText == "background translation"
	}, 4*time.Second, 100*time.Millisecond)
}

func TestInstructionTranslationProviderRejectsLooseResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"translation\":\"ok\",\"extra\":true}"}}]}`))
	}))
	t.Cleanup(server.Close)
	_, err := NewOpenAIInstructionTranslator().Translate(context.Background(), InstructionRuntimeConfig{
		ExternalTranslationEnabled: true, TranslationBaseURL: server.URL,
		TranslationModel: "translation-model", TranslationToken: "token-value",
		TranslationTimeoutMS: 1000,
	}, "external", "zh-CN", "source")
	var providerErr *instructionTranslationProviderError
	require.True(t, errors.As(err, &providerErr))
	require.Equal(t, "invalid_response", providerErr.code)
}
