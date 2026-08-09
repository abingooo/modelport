package securityaudit

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func allowedDigest(value string) instructionPolicyHash {
	return instructionPolicyHash{Digest: sha256.Sum256([]byte(value))}
}

func TestInspectInstructionPayloadUsesInstructionsThenInput1(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		allowed    []instructionPolicyHash
		wantAllow  bool
		wantReason string
		wantInstr  string
		wantInput1 string
	}{
		{
			name:       "instructions match short circuits",
			body:       `{"instructions":"trusted","input":[{}, {"content":[{"type":"input_text","text":"other"}]}]}`,
			allowed:    []instructionPolicyHash{allowedDigest("trusted")},
			wantAllow:  true,
			wantReason: "instructions_match",
			wantInstr:  "match",
			wantInput1: "not_checked",
		},
		{
			name:       "input1 fallback uses unified pool",
			body:       `{"instructions":"untrusted","input":[{}, {"content":[{"type":"input_text","text":"trust"},{"type":"input_text","text":"ed"}]}]}`,
			allowed:    []instructionPolicyHash{allowedDigest("trusted")},
			wantAllow:  true,
			wantReason: "input1_match",
			wantInstr:  "mismatch",
			wantInput1: "match",
		},
		{
			name:       "both mismatch",
			body:       `{"instructions":"one","input":[{}, {"content":[{"type":"input_text","text":"two"}]}]}`,
			allowed:    []instructionPolicyHash{allowedDigest("three")},
			wantReason: "hash_mismatch",
			wantInstr:  "mismatch",
			wantInput1: "mismatch",
		},
		{
			name:       "missing fields",
			body:       `{"model":"gpt-test"}`,
			wantReason: "fields_missing",
			wantInstr:  "missing",
			wantInput1: "missing",
		},
		{
			name:       "unsupported input1 content",
			body:       `{"input":[{}, {"content":[{"type":"input_image","image_url":"x"}]}]}`,
			wantReason: "field_invalid",
			wantInstr:  "missing",
			wantInput1: "invalid",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := inspectInstructionPayload([]byte(test.body), test.allowed)
			require.Equal(t, test.wantAllow, result.Allow)
			require.Equal(t, test.wantReason, result.Reason)
			require.Equal(t, test.wantInstr, result.Instructions.Result)
			require.Equal(t, test.wantInput1, result.Input1.Result)
		})
	}
}

func TestInspectInstructionPayloadPreservesExactText(t *testing.T) {
	allowed := []instructionPolicyHash{allowedDigest(" Line\n模型港 ")}
	result := inspectInstructionPayload([]byte(`{"instructions":" Line\n模型港 "}`), allowed)
	require.True(t, result.Allow)
	require.Equal(t, "match", result.Instructions.Result)
	require.Equal(t, " Line\n模型港 ", result.Instructions.Plaintext)

	result = inspectInstructionPayload([]byte(`{"instructions":"Line\n模型港"}`), allowed)
	require.False(t, result.Allow)
	require.Equal(t, "mismatch", result.Instructions.Result)
	require.Equal(t, "Line\n模型港", result.Instructions.Plaintext)
}

func TestStrictInstructionParserAcceptsJSONEscapedSlash(t *testing.T) {
	const trusted = "https://example.com/v1/responses"
	allowed := []instructionPolicyHash{allowedDigest(trusted)}

	result := inspectInstructionPayload([]byte(`{"instructions":"https:\/\/example.com\/v1\/responses"}`), allowed)
	require.True(t, result.Allow)
	require.Equal(t, "match", result.Instructions.Result)
	require.Equal(t, trusted, result.Instructions.Plaintext)

	result = inspectInstructionPayload([]byte(`{"instructions":"untrusted","input":[{}, {"content":[{"type":"input_text","text":"https:\/\/example.com\/v1\/responses"}]}]}`), allowed)
	require.True(t, result.Allow)
	require.Equal(t, "mismatch", result.Instructions.Result)
	require.Equal(t, "match", result.Input1.Result)
	require.Equal(t, trusted, result.Input1.Plaintext)
}

func TestStrictInstructionParserDecodesAllStandardJSONEscapes(t *testing.T) {
	const trusted = "\"\\/\b\f\n\r\tA🚀"
	result := inspectInstructionPayload(
		[]byte(`{"instructions":"\"\\\/\b\f\n\r\t\u0041\uD83D\uDE80"}`),
		[]instructionPolicyHash{allowedDigest(trusted)},
	)

	require.True(t, result.Allow)
	require.Equal(t, "match", result.Instructions.Result)
	require.Equal(t, trusted, result.Instructions.Plaintext)
}

func TestInspectInstructionPayloadFallsBackAfterInvalidInstructions(t *testing.T) {
	allowed := []instructionPolicyHash{allowedDigest("trusted")}
	for _, body := range []string{
		`{"instructions":null,"input":[{}, {"content":[{"type":"input_text","text":"trusted"}]}]}`,
		`{"instructions":"","input":[{}, {"content":[{"type":"input_text","text":"trusted"}]}]}`,
		`{"instructions":7,"input":[{}, {"content":[{"type":"input_text","text":"trusted"}]}]}`,
	} {
		result := inspectInstructionPayload([]byte(body), allowed)
		require.True(t, result.Allow)
		require.Equal(t, "invalid", result.Instructions.Result)
		require.Equal(t, "match", result.Input1.Result)
	}
}

func TestInstructionFieldsStrictlyEmpty(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "both absent", body: `{}`, want: true},
		{name: "exact empty instructions", body: `{"instructions":""}`, want: true},
		{name: "input index absent", body: `{"input":[{}]}`, want: true},
		{name: "empty content array", body: `{"input":[{}, {"content":[]}]}`, want: true},
		{name: "empty input text blocks", body: `{"instructions":"","input":[{}, {"content":[{"type":"input_text","text":""}]}]}`, want: true},
		{name: "null instructions", body: `{"instructions":null}`},
		{name: "whitespace instructions", body: `{"instructions":" "}`},
		{name: "null input", body: `{"input":null}`},
		{name: "invalid input item", body: `{"input":[{}, null]}`},
		{name: "missing content", body: `{"input":[{}, {}]}`},
		{name: "unsupported content", body: `{"input":[{}, {"content":[{"type":"input_image"}]}]}`},
		{name: "whitespace input text", body: `{"input":[{}, {"content":[{"type":"input_text","text":" "}]}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, err := decodeStrictJSONObject([]byte(test.body))
			require.NoError(t, err)
			require.Equal(t, test.want, instructionFieldsStrictlyEmpty(root))
		})
	}
}

func TestInspectInstructionPayloadRejectsStrictJSONViolations(t *testing.T) {
	for _, body := range [][]byte{
		[]byte(`{"instructions":"one","instructions":"two"}`),
		[]byte(`{"instructions":"one","instructi\u006fns":"two"}`),
		[]byte(`{"instructions":"trusted","metadata":{"duplicate":1,"duplicate":2}}`),
		[]byte("{\"instructions\":\"bad\xff\"}"),
		[]byte("{\"instructions\":\"raw\x00byte\"}"),
		[]byte(`{"instructions":"\uD800"}`),
		[]byte(`{"instructions":`),
	} {
		result := inspectInstructionPayload(body, nil)
		require.False(t, result.Allow)
		require.Equal(t, "invalid_json", result.Reason)
	}
}

func TestStrictInstructionParserTimeoutCoversLinearScans(t *testing.T) {
	deadline := time.Unix(100, 0)
	expiredNow := func() time.Time { return deadline.Add(time.Millisecond) }

	t.Run("UTF-8 validation", func(t *testing.T) {
		body := bytes.Repeat([]byte("a"), instructionAuditClockInterval+1)
		err := validateInstructionAuditUTF8(body, deadline, expiredNow)
		require.ErrorIs(t, err, errInstructionAuditParseTimeout)
	})

	t.Run("whitespace", func(t *testing.T) {
		parser := strictInstructionJSONParser{
			body:           bytes.Repeat([]byte(" "), instructionAuditClockInterval+1),
			limits:         instructionAuditParserLimits{now: expiredNow},
			deadline:       deadline,
			nextClockCheck: instructionAuditClockInterval,
		}
		require.ErrorIs(t, parser.skipWhitespace(), errInstructionAuditParseTimeout)
	})

	t.Run("number", func(t *testing.T) {
		parser := strictInstructionJSONParser{
			body:           bytes.Repeat([]byte("1"), instructionAuditClockInterval+1),
			limits:         instructionAuditParserLimits{now: expiredNow},
			deadline:       deadline,
			nextClockCheck: instructionAuditClockInterval,
		}
		require.ErrorIs(t, parser.consumeNumber(), errInstructionAuditParseTimeout)
	})

	t.Run("string decoding", func(t *testing.T) {
		body := append([]byte{'"'}, bytes.Repeat([]byte("a"), instructionAuditClockInterval+1)...)
		body = append(body, '"')
		parser := strictInstructionJSONParser{
			body:     body,
			limits:   instructionAuditParserLimits{now: expiredNow},
			deadline: deadline,
		}
		_, err := parser.decodeString(0, len(body), len(body))
		require.ErrorIs(t, err, errInstructionAuditParseTimeout)
	})
}

func TestStrictInstructionParserRetainsOnlyAuditTargets(t *testing.T) {
	body := []byte(`{"model":"gpt-test","instructions":"trusted","metadata":{"large":"ignored"},"input":[{}, {"content":[{"type":"input_text","text":"fallback","dynamic_id":"ignored"}]}]}`)
	root, err := decodeStrictJSONObject(body)
	require.NoError(t, err)
	require.Equal(t, "gpt-test", root["model"])
	require.Equal(t, "trusted", root["instructions"])
	require.NotContains(t, root, "metadata")
	input, ok := root["input"].([]any)
	require.True(t, ok)
	item, ok := input[1].(map[string]any)
	require.True(t, ok)
	content, ok := item["content"].([]any)
	require.True(t, ok)
	block, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{"type": "input_text", "text": "fallback"}, block)
}

func TestStrictInstructionParserBodyBoundaries(t *testing.T) {
	allowed := []instructionPolicyHash{allowedDigest("trusted")}
	prefix := []byte(`{"instructions":"trusted","metadata":"`)
	suffix := []byte(`"}`)
	for _, sizeMiB := range []int{1, 16, 32, 42, 64} {
		t.Run(fmt.Sprintf("%dMiB", sizeMiB), func(t *testing.T) {
			targetBytes := sizeMiB << 20
			body := make([]byte, targetBytes)
			copied := copy(body, prefix)
			for index := copied; index < targetBytes-len(suffix); index++ {
				body[index] = 'a'
			}
			copy(body[targetBytes-len(suffix):], suffix)
			limits := defaultInstructionAuditParserLimits()
			limits.ParseTimeout = 5 * time.Second
			result := inspectInstructionPayloadWithLimits(body, allowed, limits)
			require.True(t, result.Allow)
			require.Equal(t, "instructions_match", result.Reason)
		})
	}
	tooLarge := make([]byte, 65<<20)
	result := inspectInstructionPayload(tooLarge, nil)
	require.Equal(t, "request_too_large", result.Reason)
}

func TestStrictInstructionParserHonorsConfiguredTimeout(t *testing.T) {
	limits := defaultInstructionAuditParserLimits()
	limits.ParseTimeout = time.Millisecond
	clockCalls := 0
	startedAt := time.Unix(100, 0)
	limits.now = func() time.Time {
		clockCalls++
		if clockCalls > 1 {
			return startedAt.Add(2 * time.Millisecond)
		}
		return startedAt
	}
	body := []byte(`{"instructions":"trusted","metadata":"` + strings.Repeat("a", instructionAuditClockInterval*2) + `"}`)
	result := inspectInstructionPayloadWithLimits(body, nil, limits)
	require.Equal(t, "parse_timeout", result.Reason)
}

func TestInstructionParserBudgetBoundsConcurrentLargeBodies(t *testing.T) {
	const bodyBytes = 1 << 20
	prefix := []byte(`{"instructions":"trusted","metadata":"`)
	suffix := []byte(`"}`)
	body := make([]byte, bodyBytes)
	copied := copy(body, prefix)
	for index := copied; index < len(body)-len(suffix); index++ {
		body[index] = 'a'
	}
	copy(body[len(body)-len(suffix):], suffix)

	service := NewInstructionService(nil, nil, nil)
	service.configureInstructionRequestBodyBudget(int64(len(body)))
	budget := service.requestBodyBudget.Load()
	require.NotNil(t, budget)
	lease, err := budget.Acquire(context.Background(), int64(len(body)))
	require.NoError(t, err)

	runtime := InstructionRuntimeConfig{
		MaxBodyBytes:         2 << 20,
		ParseTimeoutMS:       10,
		MaxInflightBodyBytes: int64(len(body)),
	}
	_, err = service.parseInstructionRoot(context.Background(), body, runtime)
	require.ErrorIs(t, err, errInstructionAuditParseTimeout)

	lease.Release()
	runtime.ParseTimeoutMS = 5000
	root, err := service.parseInstructionRoot(context.Background(), body, runtime)
	require.NoError(t, err)
	require.Equal(t, "trusted", root["instructions"])
}

func TestInspectInstructionPayloadRejectsCompressedAndOversizedFields(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	_, err := writer.Write([]byte(`{"instructions":"trusted"}`))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	compressedResult := inspectInstructionPayload(compressed.Bytes(), []instructionPolicyHash{allowedDigest("trusted")})
	require.False(t, compressedResult.Allow)
	require.Equal(t, "invalid_json", compressedResult.Reason)

	body, err := json.Marshal(map[string]any{"instructions": strings.Repeat("a", maxInstructionAuditTextBytes+1)})
	require.NoError(t, err)
	oversizedResult := inspectInstructionPayload(body, nil)
	require.False(t, oversizedResult.Allow)
	require.Equal(t, "field_invalid", oversizedResult.Reason)
	require.Equal(t, "invalid", oversizedResult.Instructions.Result)
}

func TestInspectInstructionPayloadRejectsOversizedAndOverlyComplexBodies(t *testing.T) {
	tooLarge := make([]byte, maxInstructionAuditBodyBytes+1)
	result := inspectInstructionPayload(tooLarge, nil)
	require.False(t, result.Allow)
	require.Equal(t, "request_too_large", result.Reason)

	var body strings.Builder
	_, _ = body.WriteString(`{"metadata":[`)
	for index := 0; index < maxInstructionAuditJSONValues; index++ {
		if index > 0 {
			_ = body.WriteByte(',')
		}
		_ = body.WriteByte('0')
	}
	_, _ = body.WriteString(`]}`)
	result = inspectInstructionPayload([]byte(body.String()), nil)
	require.False(t, result.Allow)
	require.Equal(t, "structure_too_complex", result.Reason)

	deeplyNested := `{"metadata":` + strings.Repeat(`[`, maxInstructionAuditDepth+1) +
		`0` + strings.Repeat(`]`, maxInstructionAuditDepth+1) + `}`
	result = inspectInstructionPayload([]byte(deeplyNested), nil)
	require.False(t, result.Allow)
	require.Equal(t, "structure_too_complex", result.Reason)
}

func BenchmarkInspectInstructionPayload47KB(b *testing.B) {
	filler := make([]byte, 47*1024)
	for i := range filler {
		filler[i] = 'a'
	}
	body := append([]byte(`{"instructions":"trusted","metadata":"`), filler...)
	body = append(body, []byte(`"}`)...)
	allowed := []instructionPolicyHash{allowedDigest("trusted")}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = inspectInstructionPayload(body, allowed)
	}
}

func TestInstructionAudit47KBLatencyBudget(t *testing.T) {
	filler := make([]byte, 47*1024)
	for i := range filler {
		filler[i] = 'a'
	}
	body := append([]byte(`{"instructions":"trusted","metadata":"`), filler...)
	body = append(body, []byte(`"}`)...)
	allowed := []instructionPolicyHash{allowedDigest("trusted")}
	for i := 0; i < 20; i++ {
		_ = inspectInstructionPayload(body, allowed)
	}
	latencies := make([]time.Duration, 300)
	for i := range latencies {
		startedAt := time.Now()
		result := inspectInstructionPayload(body, allowed)
		latencies[i] = time.Since(startedAt)
		require.True(t, result.Allow)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95 := latencies[int(float64(len(latencies))*0.95)-1]
	p99 := latencies[int(float64(len(latencies))*0.99)-1]
	require.Less(t, p95, 10*time.Millisecond)
	require.Less(t, p99, 25*time.Millisecond)
}
