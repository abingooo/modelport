package securityaudit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

var (
	errInstructionV2InvalidJSON = errors.New("invalid instruction audit JSON")
	errInstructionV2JSONDepth   = errors.New("instruction audit JSON nesting is too deep")
)

const instructionV2MaximumJSONDepth = 1024

type instructionV2JSONCursor struct {
	ctx     context.Context
	decoder *json.Decoder
	tokens  uint64
}

func parseInstructionV2Fields(ctx context.Context, body []byte) (instructionV2ParsedFields, error) {
	fields := instructionV2ParsedFields{
		Instructions: InstructionV2Field{State: "missing"},
		Input1:       InstructionV2Field{State: "missing"},
	}
	if len(body) == 0 || !utf8.Valid(body) {
		return fields, errInstructionV2InvalidJSON
	}
	if ctx == nil {
		ctx = context.Background()
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	cursor := &instructionV2JSONCursor{ctx: ctx, decoder: decoder}
	start, err := cursor.token()
	if err != nil || start != json.Delim('{') {
		return fields, wrapInstructionV2JSONError(err)
	}
	seen := make(map[string]struct{})
	for decoder.More() {
		keyToken, tokenErr := cursor.token()
		if tokenErr != nil {
			return fields, wrapInstructionV2JSONError(tokenErr)
		}
		key, ok := keyToken.(string)
		if !ok {
			return fields, errInstructionV2InvalidJSON
		}
		if _, duplicate := seen[key]; duplicate {
			return fields, fmt.Errorf("%w: duplicate object key", errInstructionV2InvalidJSON)
		}
		seen[key] = struct{}{}
		switch key {
		case "instructions":
			fields.Instructions, err = parseInstructionV2Instructions(cursor)
		case "input":
			fields.Input1, err = parseInstructionV2Input(cursor)
		default:
			err = cursor.skipValue(1)
		}
		if err != nil {
			return fields, wrapInstructionV2JSONError(err)
		}
	}
	end, err := cursor.token()
	if err != nil || end != json.Delim('}') {
		return fields, wrapInstructionV2JSONError(err)
	}
	if err := ensureInstructionV2JSONEOF(decoder); err != nil {
		return fields, wrapInstructionV2JSONError(err)
	}
	return fields, nil
}

func parseInstructionV2Instructions(cursor *instructionV2JSONCursor) (InstructionV2Field, error) {
	token, err := cursor.token()
	if err != nil {
		return InstructionV2Field{State: "invalid"}, err
	}
	if text, ok := token.(string); ok {
		return newInstructionV2TextField(text, false), nil
	}
	if delimiter, ok := token.(json.Delim); ok {
		if err := cursor.skipDelimited(delimiter, 1); err != nil {
			return InstructionV2Field{State: "invalid"}, err
		}
	}
	return InstructionV2Field{State: "invalid"}, nil
}

func parseInstructionV2Input(cursor *instructionV2JSONCursor) (InstructionV2Field, error) {
	token, err := cursor.token()
	if err != nil {
		return InstructionV2Field{State: "invalid"}, err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter || delimiter != '[' {
		if isDelimiter {
			if err := cursor.skipDelimited(delimiter, 1); err != nil {
				return InstructionV2Field{State: "invalid"}, err
			}
		}
		return InstructionV2Field{State: "invalid"}, nil
	}

	result := InstructionV2Field{State: "missing"}
	index := 0
	for cursor.decoder.More() {
		if index == 1 {
			result, err = parseInstructionV2InputItem(cursor)
		} else {
			err = cursor.skipValue(2)
		}
		if err != nil {
			return InstructionV2Field{State: "invalid"}, err
		}
		index++
	}
	end, err := cursor.token()
	if err != nil || end != json.Delim(']') {
		return InstructionV2Field{State: "invalid"}, wrapInstructionV2JSONError(err)
	}
	return result, nil
}

func parseInstructionV2InputItem(cursor *instructionV2JSONCursor) (InstructionV2Field, error) {
	token, err := cursor.token()
	if err != nil {
		return InstructionV2Field{State: "invalid"}, err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter || delimiter != '{' {
		if isDelimiter {
			if err := cursor.skipDelimited(delimiter, 2); err != nil {
				return InstructionV2Field{State: "invalid"}, err
			}
		}
		return InstructionV2Field{State: "invalid"}, nil
	}

	result := InstructionV2Field{State: "invalid"}
	contentSeen := false
	seen := make(map[string]struct{})
	for cursor.decoder.More() {
		keyToken, tokenErr := cursor.token()
		if tokenErr != nil {
			return result, tokenErr
		}
		key, ok := keyToken.(string)
		if !ok {
			return result, errInstructionV2InvalidJSON
		}
		if _, duplicate := seen[key]; duplicate {
			return result, fmt.Errorf("%w: duplicate object key", errInstructionV2InvalidJSON)
		}
		seen[key] = struct{}{}
		if key == "content" {
			contentSeen = true
			result, err = parseInstructionV2Content(cursor)
		} else {
			err = cursor.skipValue(3)
		}
		if err != nil {
			return result, err
		}
	}
	end, err := cursor.token()
	if err != nil || end != json.Delim('}') {
		return result, wrapInstructionV2JSONError(err)
	}
	if !contentSeen {
		return InstructionV2Field{State: "invalid"}, nil
	}
	return result, nil
}

func parseInstructionV2Content(cursor *instructionV2JSONCursor) (InstructionV2Field, error) {
	token, err := cursor.token()
	if err != nil {
		return InstructionV2Field{State: "invalid"}, err
	}
	if text, ok := token.(string); ok {
		return newInstructionV2TextField(text, false), nil
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter || delimiter != '[' {
		if isDelimiter {
			if err := cursor.skipDelimited(delimiter, 3); err != nil {
				return InstructionV2Field{State: "invalid"}, err
			}
		}
		return InstructionV2Field{State: "invalid"}, nil
	}

	var builder strings.Builder
	partial := false
	invalidTextBlock := false
	for cursor.decoder.More() {
		text, recognized, valid, blockErr := parseInstructionV2ContentBlock(cursor)
		if blockErr != nil {
			return InstructionV2Field{State: "invalid"}, blockErr
		}
		if !recognized {
			partial = true
			continue
		}
		if !valid {
			invalidTextBlock = true
			continue
		}
		_, _ = builder.WriteString(text)
	}
	end, err := cursor.token()
	if err != nil || end != json.Delim(']') {
		return InstructionV2Field{State: "invalid"}, wrapInstructionV2JSONError(err)
	}
	if invalidTextBlock || partial {
		return InstructionV2Field{State: "invalid", Partial: partial || builder.Len() > 0}, nil
	}
	return newInstructionV2TextField(builder.String(), false), nil
}

func parseInstructionV2ContentBlock(cursor *instructionV2JSONCursor) (text string, recognized, valid bool, err error) {
	token, err := cursor.token()
	if err != nil {
		return "", false, false, err
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter || delimiter != '{' {
		if isDelimiter {
			if err := cursor.skipDelimited(delimiter, 4); err != nil {
				return "", false, false, err
			}
		}
		return "", false, true, nil
	}

	seen := make(map[string]struct{})
	blockType := ""
	textValue := ""
	typeValid := false
	textSeen := false
	textValid := false
	for cursor.decoder.More() {
		keyToken, tokenErr := cursor.token()
		if tokenErr != nil {
			return "", false, false, tokenErr
		}
		key, ok := keyToken.(string)
		if !ok {
			return "", false, false, errInstructionV2InvalidJSON
		}
		if _, duplicate := seen[key]; duplicate {
			return "", false, false, fmt.Errorf("%w: duplicate object key", errInstructionV2InvalidJSON)
		}
		seen[key] = struct{}{}
		switch key {
		case "type":
			value, valueErr := cursor.token()
			if valueErr != nil {
				return "", false, false, valueErr
			}
			blockType, typeValid = value.(string)
			if delimiter, ok := value.(json.Delim); ok {
				if err := cursor.skipDelimited(delimiter, 5); err != nil {
					return "", false, false, err
				}
			}
		case "text":
			textSeen = true
			value, valueErr := cursor.token()
			if valueErr != nil {
				return "", false, false, valueErr
			}
			textValue, textValid = value.(string)
			if delimiter, ok := value.(json.Delim); ok {
				if err := cursor.skipDelimited(delimiter, 5); err != nil {
					return "", false, false, err
				}
			}
		default:
			if err := cursor.skipValue(5); err != nil {
				return "", false, false, err
			}
		}
	}
	end, err := cursor.token()
	if err != nil || end != json.Delim('}') {
		return "", false, false, wrapInstructionV2JSONError(err)
	}
	if !typeValid || blockType != "input_text" {
		return "", false, true, nil
	}
	return textValue, true, textSeen && textValid, nil
}

func newInstructionV2TextField(text string, partial bool) InstructionV2Field {
	if text == "" {
		return InstructionV2Field{State: "empty", Partial: partial}
	}
	sum := sha256.Sum256([]byte(text))
	return InstructionV2Field{
		State: "valid", SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len([]byte(text))),
		Partial: partial, Plaintext: text,
	}
}

func prepareInstructionV2AISample(field InstructionV2Field, maxChars int) InstructionV2Field {
	if field.State != "valid" || field.Plaintext == "" {
		return field
	}
	if maxChars < 1 {
		maxChars = InstructionV2DefaultAIInputChars
	}
	if utf8.RuneCountInString(field.Plaintext) <= maxChars {
		field.AISample = field.Plaintext
		field.AISampled = false
		return field
	}
	firstRunes := maxChars * 2 / 3
	lastRunes := maxChars - firstRunes
	firstEnd := instructionV2ByteOffsetAfterRunes(field.Plaintext, firstRunes)
	lastStart := instructionV2ByteOffsetBeforeLastRunes(field.Plaintext, lastRunes)
	field.AISample = field.Plaintext[:firstEnd] + "\n\n[... content sampled ...]\n\n" + field.Plaintext[lastStart:]
	field.AISampled = true
	return field
}

func instructionV2ByteOffsetAfterRunes(value string, wanted int) int {
	if wanted <= 0 {
		return 0
	}
	count := 0
	for index := range value {
		if count == wanted {
			return index
		}
		count++
	}
	return len(value)
}

func instructionV2ByteOffsetBeforeLastRunes(value string, wanted int) int {
	if wanted <= 0 {
		return len(value)
	}
	total := utf8.RuneCountInString(value)
	return instructionV2ByteOffsetAfterRunes(value, total-wanted)
}

func (cursor *instructionV2JSONCursor) token() (json.Token, error) {
	if cursor == nil || cursor.decoder == nil {
		return nil, errInstructionV2InvalidJSON
	}
	cursor.tokens++
	if cursor.tokens&4095 == 0 {
		select {
		case <-cursor.ctx.Done():
			return nil, cursor.ctx.Err()
		default:
		}
	}
	return cursor.decoder.Token()
}

func (cursor *instructionV2JSONCursor) skipValue(depth int) error {
	if depth > instructionV2MaximumJSONDepth {
		return errInstructionV2JSONDepth
	}
	token, err := cursor.token()
	if err != nil {
		return err
	}
	if delimiter, ok := token.(json.Delim); ok {
		return cursor.skipDelimited(delimiter, depth)
	}
	return nil
}

func (cursor *instructionV2JSONCursor) skipDelimited(delimiter json.Delim, depth int) error {
	if depth > instructionV2MaximumJSONDepth {
		return errInstructionV2JSONDepth
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for cursor.decoder.More() {
			keyToken, err := cursor.token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errInstructionV2InvalidJSON
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("%w: duplicate object key", errInstructionV2InvalidJSON)
			}
			seen[key] = struct{}{}
			if err := cursor.skipValue(depth + 1); err != nil {
				return err
			}
		}
		end, err := cursor.token()
		if err != nil || end != json.Delim('}') {
			return wrapInstructionV2JSONError(err)
		}
	case '[':
		for cursor.decoder.More() {
			if err := cursor.skipValue(depth + 1); err != nil {
				return err
			}
		}
		end, err := cursor.token()
		if err != nil || end != json.Delim(']') {
			return wrapInstructionV2JSONError(err)
		}
	default:
		return errInstructionV2InvalidJSON
	}
	return nil
}

func ensureInstructionV2JSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("unexpected trailing JSON value")
	}
	return err
}

func wrapInstructionV2JSONError(err error) error {
	if err == nil {
		return errInstructionV2InvalidJSON
	}
	if errors.Is(err, errInstructionV2InvalidJSON) || errors.Is(err, errInstructionV2JSONDepth) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%w: %v", errInstructionV2InvalidJSON, err)
}
