package securityaudit

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxInstructionAuditBodyBytes  = 16 << 20
	maxInstructionAuditDepth      = 64
	maxInstructionAuditTextBytes  = 1 << 20
	maxInstructionAuditInputItems = 256
	maxInstructionAuditJSONValues = 64 << 10
	maxInstructionAuditContainer  = 16 << 10
	maxInstructionAuditParseTime  = 250 * time.Millisecond
)

var (
	errInstructionAuditInvalidJSON        = errors.New("invalid strict JSON")
	errInstructionAuditBodyTooLarge       = errors.New("instruction audit body too large")
	errInstructionAuditComplexityExceeded = errors.New("instruction audit JSON complexity exceeded")
	errInstructionAuditParseTimeout       = errors.New("instruction audit JSON parse timeout")
)

type instructionInspection struct {
	Instructions InstructionFieldResult
	Input1       InstructionFieldResult
	Allow        bool
	Reason       string
}

func inspectInstructionPayload(body []byte, allowed []instructionPolicyHash) instructionInspection {
	root, err := decodeStrictJSONObject(body)
	if err != nil {
		return instructionInspection{
			Instructions: InstructionFieldResult{Result: "invalid"},
			Input1:       InstructionFieldResult{Result: "not_checked"},
			Reason:       instructionAuditParseReason(err),
		}
	}
	return inspectInstructionRoot(root, allowed, time.Now().UTC())
}

func inspectInstructionRoot(root map[string]any, allowed []instructionPolicyHash, evaluatedAt time.Time) instructionInspection {
	instructions := inspectInstructions(root, allowed, evaluatedAt)
	if instructions.Result == "match" {
		return instructionInspection{
			Instructions: instructions,
			Input1:       InstructionFieldResult{Result: "not_checked"},
			Allow:        true,
			Reason:       "instructions_match",
		}
	}

	input1 := inspectInput1(root, allowed, evaluatedAt)
	if input1.Result == "match" {
		return instructionInspection{
			Instructions: instructions,
			Input1:       input1,
			Allow:        true,
			Reason:       "input1_match",
		}
	}

	reason := "hash_mismatch"
	if instructions.Result == "missing" && input1.Result == "missing" {
		reason = "fields_missing"
	} else if instructions.Result == "invalid" || input1.Result == "invalid" {
		reason = "field_invalid"
	}
	return instructionInspection{Instructions: instructions, Input1: input1, Reason: reason}
}

func strictInstructionModel(root map[string]any) (string, bool) {
	model, ok := root["model"].(string)
	model = normalizeInstructionAuditModel(model)
	return model, ok && model != ""
}

func lenientLastInstructionModel(body []byte) string {
	var envelope struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return ""
	}
	return normalizeInstructionAuditModel(envelope.Model)
}

func inspectInstructions(root map[string]any, allowed []instructionPolicyHash, evaluatedAt time.Time) InstructionFieldResult {
	value, exists := root["instructions"]
	if !exists {
		return InstructionFieldResult{Result: "missing"}
	}
	result := InstructionFieldResult{Present: true, Result: "invalid"}
	text, ok := value.(string)
	if !ok || text == "" || len(text) > maxInstructionAuditTextBytes {
		return result
	}
	result.SHA256 = sha256Hex(text)
	result.Result = "mismatch"
	if matchesInstructionDigest(result.SHA256, allowed, evaluatedAt) {
		result.Result = "match"
	}
	return result
}

func inspectInput1(root map[string]any, allowed []instructionPolicyHash, evaluatedAt time.Time) InstructionFieldResult {
	value, exists := root["input"]
	if !exists {
		return InstructionFieldResult{Result: "missing"}
	}
	input, ok := value.([]any)
	if !ok || len(input) <= 1 {
		return InstructionFieldResult{Present: len(input) > 1, Result: "invalid"}
	}
	result := InstructionFieldResult{Present: true, Result: "invalid"}
	item, ok := input[1].(map[string]any)
	if !ok {
		return result
	}
	contentValue, exists := item["content"]
	if !exists {
		return result
	}
	content, ok := contentValue.([]any)
	if !ok || len(content) == 0 || len(content) > maxInstructionAuditInputItems {
		return result
	}

	var builder strings.Builder
	for _, rawBlock := range content {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			return result
		}
		blockType, ok := block["type"].(string)
		if !ok || blockType != "input_text" {
			return result
		}
		text, ok := block["text"].(string)
		if !ok || builder.Len()+len(text) > maxInstructionAuditTextBytes {
			return result
		}
		builder.WriteString(text)
	}
	if builder.Len() == 0 {
		return result
	}
	result.SHA256 = sha256Hex(builder.String())
	result.Result = "mismatch"
	if matchesInstructionDigest(result.SHA256, allowed, evaluatedAt) {
		result.Result = "match"
	}
	return result
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func matchesInstructionDigest(digest string, allowed []instructionPolicyHash, evaluatedAt time.Time) bool {
	decoded, err := hex.DecodeString(digest)
	if err != nil || len(decoded) != sha256.Size {
		return false
	}
	matched := 0
	for i := range allowed {
		if !allowed[i].ValidFrom.IsZero() && evaluatedAt.Before(allowed[i].ValidFrom) {
			continue
		}
		if !allowed[i].ValidUntil.IsZero() && !evaluatedAt.Before(allowed[i].ValidUntil) {
			continue
		}
		matched |= subtle.ConstantTimeCompare(decoded, allowed[i].Digest[:])
	}
	return matched == 1
}

func instructionAuditParseReason(err error) string {
	switch {
	case errors.Is(err, errInstructionAuditBodyTooLarge):
		return "request_too_large"
	case errors.Is(err, errInstructionAuditComplexityExceeded):
		return "structure_too_complex"
	case errors.Is(err, errInstructionAuditParseTimeout):
		return "parse_timeout"
	default:
		return "invalid_json"
	}
}

func decodeStrictJSONObject(body []byte) (map[string]any, error) {
	if len(body) == 0 {
		return nil, errInstructionAuditInvalidJSON
	}
	if len(body) > maxInstructionAuditBodyBytes {
		return nil, errInstructionAuditBodyTooLarge
	}
	if !utf8.Valid(body) {
		return nil, errInstructionAuditInvalidJSON
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	state := strictJSONDecodeState{deadline: time.Now().Add(maxInstructionAuditParseTime)}
	value, err := decodeStrictJSONValue(decoder, 0, &state)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errInstructionAuditInvalidJSON
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, errInstructionAuditInvalidJSON
	}
	return root, nil
}

type strictJSONDecodeState struct {
	values   int
	deadline time.Time
}

func decodeStrictJSONValue(decoder *json.Decoder, depth int, state *strictJSONDecodeState) (any, error) {
	if depth > maxInstructionAuditDepth {
		return nil, fmt.Errorf("%w: nesting too deep", errInstructionAuditInvalidJSON)
	}
	state.values++
	if state.values > maxInstructionAuditJSONValues {
		return nil, errInstructionAuditComplexityExceeded
	}
	if state.values%256 == 0 && time.Now().After(state.deadline) {
		return nil, errInstructionAuditParseTimeout
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errInstructionAuditInvalidJSON, err)
	}
	if time.Now().After(state.deadline) {
		return nil, errInstructionAuditParseTimeout
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return token, nil
	}
	switch delim {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			if len(object) >= maxInstructionAuditContainer {
				return nil, errInstructionAuditComplexityExceeded
			}
			keyToken, keyErr := decoder.Token()
			if keyErr != nil {
				return nil, fmt.Errorf("%w: %v", errInstructionAuditInvalidJSON, keyErr)
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errInstructionAuditInvalidJSON
			}
			if _, duplicate := object[key]; duplicate {
				return nil, fmt.Errorf("%w: duplicate key", errInstructionAuditInvalidJSON)
			}
			value, valueErr := decodeStrictJSONValue(decoder, depth+1, state)
			if valueErr != nil {
				return nil, valueErr
			}
			object[key] = value
		}
		closing, closeErr := decoder.Token()
		if closeErr != nil || closing != json.Delim('}') {
			return nil, errInstructionAuditInvalidJSON
		}
		return object, nil
	case '[':
		array := make([]any, 0)
		for decoder.More() {
			if len(array) >= maxInstructionAuditContainer {
				return nil, errInstructionAuditComplexityExceeded
			}
			value, valueErr := decodeStrictJSONValue(decoder, depth+1, state)
			if valueErr != nil {
				return nil, valueErr
			}
			array = append(array, value)
		}
		closing, closeErr := decoder.Token()
		if closeErr != nil || closing != json.Delim(']') {
			return nil, errInstructionAuditInvalidJSON
		}
		return array, nil
	default:
		return nil, errInstructionAuditInvalidJSON
	}
}
