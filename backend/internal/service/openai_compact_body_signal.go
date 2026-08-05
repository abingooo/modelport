package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

const (
	maxOpenAICompactSignalBodyBytes = 16 << 20
	maxOpenAICompactSignalDepth     = 64
	maxOpenAICompactSignalValues    = 64 << 10
	maxOpenAICompactSignalContainer = 16 << 10
)

var errInvalidOpenAICompactSignalJSON = errors.New("invalid compact signal JSON")

// HasCompactionTriggerInInput detects an input item with
// type="compaction_trigger". The handler combines this body signal with the
// request path, stream flag, and Codex beta feature header to distinguish the
// native remote compaction v2 wire from the legacy /responses/compact bridge.
func HasCompactionTriggerInInput(body []byte) bool {
	if len(body) == 0 || len(body) > maxOpenAICompactSignalBodyBytes || !utf8.Valid(body) {
		return false
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false
	}
	candidate := false
	input.ForEach(func(_, item gjson.Result) bool {
		if item.Get("type").String() == "compaction_trigger" {
			candidate = true
			return false
		}
		return true
	})
	if !candidate {
		return false
	}

	root, err := decodeStrictOpenAICompactSignalObject(body)
	if err != nil {
		return false
	}
	items, ok := root["input"].([]any)
	if !ok || len(items) == 0 {
		return false
	}
	found := false
	for index, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		itemType, _ := item["type"].(string)
		if itemType != "compaction_trigger" {
			continue
		}
		if found || index != len(items)-1 || len(item) != 1 {
			return false
		}
		found = true
	}
	return found
}

type openAICompactSignalDecodeState struct {
	values int
}

func decodeStrictOpenAICompactSignalObject(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	state := openAICompactSignalDecodeState{}
	value, err := decodeStrictOpenAICompactSignalValue(decoder, 0, &state)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errInvalidOpenAICompactSignalJSON
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, errInvalidOpenAICompactSignalJSON
	}
	return root, nil
}

func decodeStrictOpenAICompactSignalValue(decoder *json.Decoder, depth int, state *openAICompactSignalDecodeState) (any, error) {
	if depth > maxOpenAICompactSignalDepth {
		return nil, errInvalidOpenAICompactSignalJSON
	}
	state.values++
	if state.values > maxOpenAICompactSignalValues {
		return nil, errInvalidOpenAICompactSignalJSON
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errInvalidOpenAICompactSignalJSON, err)
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return token, nil
	}
	switch delim {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			if len(object) >= maxOpenAICompactSignalContainer {
				return nil, errInvalidOpenAICompactSignalJSON
			}
			keyToken, keyErr := decoder.Token()
			if keyErr != nil {
				return nil, errInvalidOpenAICompactSignalJSON
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errInvalidOpenAICompactSignalJSON
			}
			if _, duplicate := object[key]; duplicate {
				return nil, errInvalidOpenAICompactSignalJSON
			}
			value, valueErr := decodeStrictOpenAICompactSignalValue(decoder, depth+1, state)
			if valueErr != nil {
				return nil, valueErr
			}
			object[key] = value
		}
		closing, closeErr := decoder.Token()
		if closeErr != nil || closing != json.Delim('}') {
			return nil, errInvalidOpenAICompactSignalJSON
		}
		return object, nil
	case '[':
		array := make([]any, 0)
		for decoder.More() {
			if len(array) >= maxOpenAICompactSignalContainer {
				return nil, errInvalidOpenAICompactSignalJSON
			}
			value, valueErr := decodeStrictOpenAICompactSignalValue(decoder, depth+1, state)
			if valueErr != nil {
				return nil, valueErr
			}
			array = append(array, value)
		}
		closing, closeErr := decoder.Token()
		if closeErr != nil || closing != json.Delim(']') {
			return nil, errInvalidOpenAICompactSignalJSON
		}
		return array, nil
	default:
		return nil, errInvalidOpenAICompactSignalJSON
	}
}
