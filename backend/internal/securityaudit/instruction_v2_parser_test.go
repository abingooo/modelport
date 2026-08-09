package securityaudit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func TestInstructionV2ParserHashesExactDecodedText(t *testing.T) {
	text := "  System prompt\n\t\u4f60\u597d e\u0301 🚀  "
	body := []byte(`{"instructions":"  System prompt\n\t\u4f60\u597d e\u0301 🚀  ","input":[]}`)

	fields, err := parseInstructionV2Fields(context.Background(), body)
	require.NoError(t, err)
	require.Equal(t, "valid", fields.Instructions.State)
	require.Equal(t, text, fields.Instructions.Plaintext)
	require.Equal(t, int64(len([]byte(text))), fields.Instructions.Bytes)
	require.Equal(t, instructionV2TestDigest(text), fields.Instructions.SHA256)
	require.Equal(t, "missing", fields.Input1.State)
}

func TestInstructionV2ParserJoinsInput1TextBlocksInOrder(t *testing.T) {
	body := []byte(`{
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"ignored"}]},
			{"role":"developer","content":[
				{"text":"first ","type":"input_text"},
				{"type":"input_text","text":"\u4e8c"},
				{"type":"input_text","text":"\nthird"}
			]}
		]
	}`)

	fields, err := parseInstructionV2Fields(context.Background(), body)
	require.NoError(t, err)
	require.Equal(t, "missing", fields.Instructions.State)
	require.Equal(t, "valid", fields.Input1.State)
	require.Equal(t, "first \u4e8c\nthird", fields.Input1.Plaintext)
	require.Equal(t, instructionV2TestDigest("first \u4e8c\nthird"), fields.Input1.SHA256)
}

func TestInstructionV2ParserFieldStates(t *testing.T) {
	tests := []struct {
		name              string
		body              string
		instructionsState string
		input1State       string
	}{
		{name: "missing", body: `{}`, instructionsState: "missing", input1State: "missing"},
		{name: "empty", body: `{"instructions":"","input":[{}, {"content":[]}]}`, instructionsState: "empty", input1State: "empty"},
		{name: "null instructions", body: `{"instructions":null}`, instructionsState: "invalid", input1State: "missing"},
		{name: "short input", body: `{"input":[{"content":"first"}]}`, instructionsState: "missing", input1State: "missing"},
		{name: "invalid input shape", body: `{"input":{"content":[]}}`, instructionsState: "missing", input1State: "invalid"},
		{name: "invalid input item", body: `{"input":[{},"text"]}`, instructionsState: "missing", input1State: "invalid"},
		{name: "missing content", body: `{"input":[{},{}]}`, instructionsState: "missing", input1State: "invalid"},
		{name: "invalid text block", body: `{"input":[{}, {"content":[{"type":"input_text","text":7}]}]}`, instructionsState: "missing", input1State: "invalid"},
		{name: "unsupported block", body: `{"input":[{}, {"content":[{"type":"input_image","image_url":"data:"}]}]}`, instructionsState: "missing", input1State: "invalid"},
		{name: "mixed block types", body: `{"input":[{}, {"content":[{"type":"input_text","text":"known"},{"type":"input_image","image_url":"data:"}]}]}`, instructionsState: "missing", input1State: "invalid"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields, err := parseInstructionV2Fields(context.Background(), []byte(test.body))
			require.NoError(t, err)
			require.Equal(t, test.instructionsState, fields.Instructions.State)
			require.Equal(t, test.input1State, fields.Input1.State)
		})
	}
}

func TestInstructionV2ParserRejectsMalformedJSON(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{name: "empty body", body: nil},
		{name: "invalid UTF-8", body: []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}},
		{name: "duplicate top-level key", body: []byte(`{"instructions":"a","instructions":"b"}`)},
		{name: "duplicate nested key", body: []byte(`{"input":[{}, {"content":[{"type":"input_text","text":"a","text":"b"}]}]}`)},
		{name: "trailing JSON", body: []byte(`{"instructions":"a"} {"instructions":"b"}`)},
		{name: "trailing token", body: []byte(`{"instructions":"a"}x`)},
		{name: "top-level array", body: []byte(`[]`)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseInstructionV2Fields(context.Background(), test.body)
			require.Error(t, err)
			require.ErrorIs(t, err, errInstructionV2InvalidJSON)
		})
	}
}

func TestInstructionV2ParserRejectsExcessiveDepth(t *testing.T) {
	body := `{"metadata":` + strings.Repeat("[", instructionV2MaximumJSONDepth+1) + `0` + strings.Repeat("]", instructionV2MaximumJSONDepth+1) + `}`
	_, err := parseInstructionV2Fields(context.Background(), []byte(body))
	require.ErrorIs(t, err, errInstructionV2JSONDepth)
}

func TestInstructionV2ParserHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	body := []byte(`{"metadata":[` + strings.Repeat(`{"value":1},`, 5000) + `null]}`)

	_, err := parseInstructionV2Fields(ctx, body)
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled), err)
}

func TestInstructionV2AISamplingUsesFirstTwoThirdsAndFinalThird(t *testing.T) {
	const maximum = 64000
	text := strings.Repeat("a", maximum*2/3) + strings.Repeat("界", maximum-maximum*2/3) + strings.Repeat("z", 100)
	field := newInstructionV2TextField(text, false)

	sampled := prepareInstructionV2AISample(field, maximum)
	require.True(t, sampled.AISampled)
	parts := strings.Split(sampled.AISample, "\n\n[... content sampled ...]\n\n")
	require.Len(t, parts, 2)
	require.Equal(t, maximum*2/3, utf8.RuneCountInString(parts[0]))
	require.Equal(t, maximum-maximum*2/3, utf8.RuneCountInString(parts[1]))
	require.Equal(t, text[:instructionV2ByteOffsetAfterRunes(text, maximum*2/3)], parts[0])
	require.Equal(t, text[instructionV2ByteOffsetBeforeLastRunes(text, maximum-maximum*2/3):], parts[1])
	require.Equal(t, instructionV2TestDigest(text), sampled.SHA256)
	require.Equal(t, text, sampled.Plaintext)
}

func TestInstructionV2AISamplingKeepsShortTextUnchanged(t *testing.T) {
	field := prepareInstructionV2AISample(newInstructionV2TextField("短文本", false), 64000)
	require.False(t, field.AISampled)
	require.Equal(t, "短文本", field.AISample)
}

func instructionV2TestDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
