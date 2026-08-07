package securityaudit

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxInstructionAuditBodyBytes  = 64 << 20
	maxInstructionAuditDepth      = 64
	maxInstructionAuditTextBytes  = 1 << 20
	maxInstructionAuditKeyBytes   = 1 << 20
	maxInstructionAuditInputItems = 256
	maxInstructionAuditJSONValues = 64 << 10
	maxInstructionAuditContainer  = 16 << 10
	maxInstructionAuditParseTime  = 500 * time.Millisecond
	instructionAuditClockInterval = 4 << 10
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

type instructionAuditParserLimits struct {
	MaxBodyBytes      int
	MaxDepth          int
	MaxTextBytes      int
	MaxKeyBytes       int
	MaxInputItems     int
	MaxJSONValues     int
	MaxContainerItems int
	ParseTimeout      time.Duration
	now               func() time.Time
}

func defaultInstructionAuditParserLimits() instructionAuditParserLimits {
	return instructionAuditParserLimits{
		MaxBodyBytes:      maxInstructionAuditBodyBytes,
		MaxDepth:          maxInstructionAuditDepth,
		MaxTextBytes:      maxInstructionAuditTextBytes,
		MaxKeyBytes:       maxInstructionAuditKeyBytes,
		MaxInputItems:     maxInstructionAuditInputItems,
		MaxJSONValues:     maxInstructionAuditJSONValues,
		MaxContainerItems: maxInstructionAuditContainer,
		ParseTimeout:      maxInstructionAuditParseTime,
		now:               time.Now,
	}
}

func (limits instructionAuditParserLimits) normalized() instructionAuditParserLimits {
	defaults := defaultInstructionAuditParserLimits()
	if limits.MaxBodyBytes <= 0 {
		limits.MaxBodyBytes = defaults.MaxBodyBytes
	}
	if limits.MaxDepth <= 0 {
		limits.MaxDepth = defaults.MaxDepth
	}
	if limits.MaxTextBytes <= 0 {
		limits.MaxTextBytes = defaults.MaxTextBytes
	}
	if limits.MaxKeyBytes <= 0 {
		limits.MaxKeyBytes = defaults.MaxKeyBytes
	}
	if limits.MaxInputItems <= 0 {
		limits.MaxInputItems = defaults.MaxInputItems
	}
	if limits.MaxJSONValues <= 0 {
		limits.MaxJSONValues = defaults.MaxJSONValues
	}
	if limits.MaxContainerItems <= 0 {
		limits.MaxContainerItems = defaults.MaxContainerItems
	}
	if limits.ParseTimeout <= 0 {
		limits.ParseTimeout = defaults.ParseTimeout
	}
	if limits.now == nil {
		limits.now = defaults.now
	}
	return limits
}

func inspectInstructionPayload(body []byte, allowed []instructionPolicyHash) instructionInspection {
	return inspectInstructionPayloadWithLimits(body, allowed, defaultInstructionAuditParserLimits())
}

func inspectInstructionPayloadWithLimits(body []byte, allowed []instructionPolicyHash, limits instructionAuditParserLimits) instructionInspection {
	root, err := decodeStrictJSONObjectWithLimits(body, limits)
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

func instructionFieldsStrictlyEmpty(root map[string]any) bool {
	if root == nil {
		return false
	}
	if value, exists := root["instructions"]; exists {
		text, ok := value.(string)
		if !ok || text != "" {
			return false
		}
	}

	value, exists := root["input"]
	if !exists {
		return true
	}
	input, ok := value.([]any)
	if !ok {
		return false
	}
	if len(input) <= 1 {
		return true
	}
	item, ok := input[1].(map[string]any)
	if !ok {
		return false
	}
	contentValue, exists := item["content"]
	if !exists {
		return false
	}
	content, ok := contentValue.([]any)
	if !ok || len(content) > maxInstructionAuditInputItems {
		return false
	}
	for _, rawBlock := range content {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			return false
		}
		blockType, ok := block["type"].(string)
		if !ok || blockType != "input_text" {
			return false
		}
		text, ok := block["text"].(string)
		if !ok || text != "" {
			return false
		}
	}
	return true
}

func strictInstructionModel(root map[string]any) (string, bool) {
	model, ok := root["model"].(string)
	model = strings.TrimSpace(model)
	return model, ok && model != ""
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
	result.Plaintext = text
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
		_, _ = builder.WriteString(text)
	}
	if builder.Len() == 0 {
		return result
	}
	result.Plaintext = builder.String()
	result.SHA256 = sha256Hex(result.Plaintext)
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
	return decodeStrictJSONObjectWithLimits(body, defaultInstructionAuditParserLimits())
}

func decodeStrictJSONObjectWithLimits(body []byte, limits instructionAuditParserLimits) (map[string]any, error) {
	limits = limits.normalized()
	if len(body) == 0 {
		return nil, errInstructionAuditInvalidJSON
	}
	if len(body) > limits.MaxBodyBytes {
		return nil, errInstructionAuditBodyTooLarge
	}
	if !utf8.Valid(body) {
		return nil, errInstructionAuditInvalidJSON
	}
	parser := strictInstructionJSONParser{
		body: body, limits: limits, deadline: limits.now().Add(limits.ParseTimeout),
		nextClockCheck: instructionAuditClockInterval,
	}
	root, err := parser.parseRoot()
	if err != nil {
		return nil, err
	}
	parser.skipWhitespace()
	if parser.position != len(body) {
		return nil, errInstructionAuditInvalidJSON
	}
	return root, nil
}

type instructionInvalidJSONValue struct{}

type strictInstructionJSONParser struct {
	body           []byte
	position       int
	values         int
	limits         instructionAuditParserLimits
	deadline       time.Time
	nextClockCheck int
}

func (parser *strictInstructionJSONParser) parseRoot() (map[string]any, error) {
	if err := parser.beginValue(0); err != nil {
		return nil, err
	}
	parser.skipWhitespace()
	if !parser.consumeByte('{') {
		return nil, errInstructionAuditInvalidJSON
	}
	result := make(map[string]any, 3)
	seen := make(map[string]struct{})
	count := 0
	parser.skipWhitespace()
	if parser.consumeByte('}') {
		return result, nil
	}
	for {
		count++
		if count > parser.limits.MaxContainerItems {
			return nil, errInstructionAuditComplexityExceeded
		}
		key, err := parser.parseObjectKey(seen)
		if err != nil {
			return nil, err
		}
		parser.skipWhitespace()
		if !parser.consumeByte(':') {
			return nil, errInstructionAuditInvalidJSON
		}
		switch key {
		case "instructions", "model":
			result[key], err = parser.parseCapturedStringOrInvalid(1, parser.limits.MaxTextBytes)
		case "input":
			result[key], err = parser.parseInput(1)
		default:
			err = parser.skipValue(1)
		}
		if err != nil {
			return nil, err
		}
		parser.skipWhitespace()
		if parser.consumeByte('}') {
			return result, nil
		}
		if !parser.consumeByte(',') {
			return nil, errInstructionAuditInvalidJSON
		}
	}
}

func (parser *strictInstructionJSONParser) parseInput(depth int) (any, error) {
	parser.skipWhitespace()
	if parser.peekByte() != '[' {
		if err := parser.skipValue(depth); err != nil {
			return nil, err
		}
		return instructionInvalidJSONValue{}, nil
	}
	if err := parser.beginValue(depth); err != nil {
		return nil, err
	}
	parser.position++
	result := make([]any, 0, 2)
	count := 0
	parser.skipWhitespace()
	if parser.consumeByte(']') {
		return result, nil
	}
	for {
		if count >= parser.limits.MaxContainerItems {
			return nil, errInstructionAuditComplexityExceeded
		}
		if count == 1 {
			item, err := parser.parseInputItem(depth + 1)
			if err != nil {
				return nil, err
			}
			result = append(result, item)
		} else {
			if err := parser.skipValue(depth + 1); err != nil {
				return nil, err
			}
			if count == 0 {
				result = append(result, nil)
			}
		}
		count++
		parser.skipWhitespace()
		if parser.consumeByte(']') {
			return result, nil
		}
		if !parser.consumeByte(',') {
			return nil, errInstructionAuditInvalidJSON
		}
	}
}

func (parser *strictInstructionJSONParser) parseInputItem(depth int) (any, error) {
	parser.skipWhitespace()
	if parser.peekByte() != '{' {
		if err := parser.skipValue(depth); err != nil {
			return nil, err
		}
		return instructionInvalidJSONValue{}, nil
	}
	if err := parser.beginValue(depth); err != nil {
		return nil, err
	}
	parser.position++
	result := make(map[string]any, 1)
	seen := make(map[string]struct{})
	count := 0
	parser.skipWhitespace()
	if parser.consumeByte('}') {
		return result, nil
	}
	for {
		count++
		if count > parser.limits.MaxContainerItems {
			return nil, errInstructionAuditComplexityExceeded
		}
		key, err := parser.parseObjectKey(seen)
		if err != nil {
			return nil, err
		}
		parser.skipWhitespace()
		if !parser.consumeByte(':') {
			return nil, errInstructionAuditInvalidJSON
		}
		if key == "content" {
			result[key], err = parser.parseInputContent(depth + 1)
		} else {
			err = parser.skipValue(depth + 1)
		}
		if err != nil {
			return nil, err
		}
		parser.skipWhitespace()
		if parser.consumeByte('}') {
			return result, nil
		}
		if !parser.consumeByte(',') {
			return nil, errInstructionAuditInvalidJSON
		}
	}
}

func (parser *strictInstructionJSONParser) parseInputContent(depth int) (any, error) {
	parser.skipWhitespace()
	if parser.peekByte() != '[' {
		if err := parser.skipValue(depth); err != nil {
			return nil, err
		}
		return instructionInvalidJSONValue{}, nil
	}
	if err := parser.beginValue(depth); err != nil {
		return nil, err
	}
	parser.position++
	blocks := make([]any, 0)
	count := 0
	tooMany := false
	parser.skipWhitespace()
	if parser.consumeByte(']') {
		return blocks, nil
	}
	for {
		if count >= parser.limits.MaxContainerItems {
			return nil, errInstructionAuditComplexityExceeded
		}
		block, err := parser.parseInputContentBlock(depth + 1)
		if err != nil {
			return nil, err
		}
		if count < parser.limits.MaxInputItems {
			blocks = append(blocks, block)
		} else {
			tooMany = true
		}
		count++
		parser.skipWhitespace()
		if parser.consumeByte(']') {
			if tooMany {
				return instructionInvalidJSONValue{}, nil
			}
			return blocks, nil
		}
		if !parser.consumeByte(',') {
			return nil, errInstructionAuditInvalidJSON
		}
	}
}

func (parser *strictInstructionJSONParser) parseInputContentBlock(depth int) (any, error) {
	parser.skipWhitespace()
	if parser.peekByte() != '{' {
		if err := parser.skipValue(depth); err != nil {
			return nil, err
		}
		return instructionInvalidJSONValue{}, nil
	}
	if err := parser.beginValue(depth); err != nil {
		return nil, err
	}
	parser.position++
	result := make(map[string]any, 2)
	seen := make(map[string]struct{})
	count := 0
	parser.skipWhitespace()
	if parser.consumeByte('}') {
		return result, nil
	}
	for {
		count++
		if count > parser.limits.MaxContainerItems {
			return nil, errInstructionAuditComplexityExceeded
		}
		key, err := parser.parseObjectKey(seen)
		if err != nil {
			return nil, err
		}
		parser.skipWhitespace()
		if !parser.consumeByte(':') {
			return nil, errInstructionAuditInvalidJSON
		}
		if key == "type" || key == "text" {
			result[key], err = parser.parseCapturedStringOrInvalid(depth+1, parser.limits.MaxTextBytes)
		} else {
			err = parser.skipValue(depth + 1)
		}
		if err != nil {
			return nil, err
		}
		parser.skipWhitespace()
		if parser.consumeByte('}') {
			return result, nil
		}
		if !parser.consumeByte(',') {
			return nil, errInstructionAuditInvalidJSON
		}
	}
}

func (parser *strictInstructionJSONParser) parseCapturedStringOrInvalid(depth, maxBytes int) (any, error) {
	parser.skipWhitespace()
	if parser.peekByte() != '"' {
		if err := parser.skipValue(depth); err != nil {
			return nil, err
		}
		return instructionInvalidJSONValue{}, nil
	}
	if err := parser.beginValue(depth); err != nil {
		return nil, err
	}
	start, end, err := parser.scanString()
	if err != nil {
		return nil, err
	}
	value, err := parser.decodeString(start, end, maxBytes)
	if errors.Is(err, errInstructionAuditComplexityExceeded) {
		return instructionInvalidJSONValue{}, nil
	}
	return value, err
}

func (parser *strictInstructionJSONParser) skipValue(depth int) error {
	if err := parser.beginValue(depth); err != nil {
		return err
	}
	parser.skipWhitespace()
	switch parser.peekByte() {
	case '"':
		_, _, err := parser.scanString()
		return err
	case '{':
		parser.position++
		seen := make(map[string]struct{})
		count := 0
		parser.skipWhitespace()
		if parser.consumeByte('}') {
			return nil
		}
		for {
			count++
			if count > parser.limits.MaxContainerItems {
				return errInstructionAuditComplexityExceeded
			}
			if _, err := parser.parseObjectKey(seen); err != nil {
				return err
			}
			parser.skipWhitespace()
			if !parser.consumeByte(':') {
				return errInstructionAuditInvalidJSON
			}
			if err := parser.skipValue(depth + 1); err != nil {
				return err
			}
			parser.skipWhitespace()
			if parser.consumeByte('}') {
				return nil
			}
			if !parser.consumeByte(',') {
				return errInstructionAuditInvalidJSON
			}
		}
	case '[':
		parser.position++
		count := 0
		parser.skipWhitespace()
		if parser.consumeByte(']') {
			return nil
		}
		for {
			count++
			if count > parser.limits.MaxContainerItems {
				return errInstructionAuditComplexityExceeded
			}
			if err := parser.skipValue(depth + 1); err != nil {
				return err
			}
			parser.skipWhitespace()
			if parser.consumeByte(']') {
				return nil
			}
			if !parser.consumeByte(',') {
				return errInstructionAuditInvalidJSON
			}
		}
	case 't':
		return parser.consumeLiteral("true")
	case 'f':
		return parser.consumeLiteral("false")
	case 'n':
		return parser.consumeLiteral("null")
	default:
		return parser.consumeNumber()
	}
}

func (parser *strictInstructionJSONParser) parseObjectKey(seen map[string]struct{}) (string, error) {
	parser.skipWhitespace()
	if parser.peekByte() != '"' {
		return "", errInstructionAuditInvalidJSON
	}
	start, end, err := parser.scanString()
	if err != nil {
		return "", err
	}
	key, err := parser.decodeString(start, end, parser.limits.MaxKeyBytes)
	if err != nil {
		return "", err
	}
	if _, duplicate := seen[key]; duplicate {
		return "", fmt.Errorf("%w: duplicate key", errInstructionAuditInvalidJSON)
	}
	seen[key] = struct{}{}
	return key, nil
}

func (parser *strictInstructionJSONParser) scanString() (int, int, error) {
	if !parser.consumeByte('"') {
		return 0, 0, errInstructionAuditInvalidJSON
	}
	start := parser.position - 1
	for parser.position < len(parser.body) {
		if err := parser.checkDeadline(); err != nil {
			return 0, 0, err
		}
		value := parser.body[parser.position]
		parser.position++
		switch value {
		case '"':
			return start, parser.position, nil
		case '\\':
			if parser.position >= len(parser.body) {
				return 0, 0, errInstructionAuditInvalidJSON
			}
			escaped := parser.body[parser.position]
			parser.position++
			switch escaped {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
			case 'u':
				codePoint, err := parser.consumeUnicodeEscape()
				if err != nil {
					return 0, 0, err
				}
				if codePoint >= 0xD800 && codePoint <= 0xDBFF {
					if parser.position+2 > len(parser.body) || parser.body[parser.position] != '\\' || parser.body[parser.position+1] != 'u' {
						return 0, 0, errInstructionAuditInvalidJSON
					}
					parser.position += 2
					low, lowErr := parser.consumeUnicodeEscape()
					if lowErr != nil || low < 0xDC00 || low > 0xDFFF {
						return 0, 0, errInstructionAuditInvalidJSON
					}
				} else if codePoint >= 0xDC00 && codePoint <= 0xDFFF {
					return 0, 0, errInstructionAuditInvalidJSON
				}
			default:
				return 0, 0, errInstructionAuditInvalidJSON
			}
		default:
			if value < 0x20 {
				return 0, 0, errInstructionAuditInvalidJSON
			}
		}
	}
	return 0, 0, errInstructionAuditInvalidJSON
}

func (parser *strictInstructionJSONParser) consumeUnicodeEscape() (int, error) {
	if parser.position+4 > len(parser.body) {
		return 0, errInstructionAuditInvalidJSON
	}
	value := 0
	for range 4 {
		digit := hexDigitValue(parser.body[parser.position])
		if digit < 0 {
			return 0, errInstructionAuditInvalidJSON
		}
		value = value*16 + digit
		parser.position++
	}
	return value, nil
}

func hexDigitValue(value byte) int {
	switch {
	case value >= '0' && value <= '9':
		return int(value - '0')
	case value >= 'a' && value <= 'f':
		return int(value-'a') + 10
	case value >= 'A' && value <= 'F':
		return int(value-'A') + 10
	default:
		return -1
	}
}

func (parser *strictInstructionJSONParser) decodeString(start, end, maxBytes int) (string, error) {
	if end-start > maxBytes*6+2 {
		return "", errInstructionAuditComplexityExceeded
	}
	value, err := strconv.Unquote(string(parser.body[start:end]))
	if err != nil {
		return "", errInstructionAuditInvalidJSON
	}
	if len(value) > maxBytes {
		return "", errInstructionAuditComplexityExceeded
	}
	return value, nil
}

func (parser *strictInstructionJSONParser) consumeLiteral(literal string) error {
	if parser.position+len(literal) > len(parser.body) || string(parser.body[parser.position:parser.position+len(literal)]) != literal {
		return errInstructionAuditInvalidJSON
	}
	parser.position += len(literal)
	if !parser.atValueBoundary() {
		return errInstructionAuditInvalidJSON
	}
	return nil
}

func (parser *strictInstructionJSONParser) consumeNumber() error {
	start := parser.position
	if parser.consumeByte('-') && parser.position >= len(parser.body) {
		return errInstructionAuditInvalidJSON
	}
	if parser.consumeByte('0') {
		if parser.position < len(parser.body) && parser.body[parser.position] >= '0' && parser.body[parser.position] <= '9' {
			return errInstructionAuditInvalidJSON
		}
	} else {
		if parser.position >= len(parser.body) || parser.body[parser.position] < '1' || parser.body[parser.position] > '9' {
			return errInstructionAuditInvalidJSON
		}
		for parser.position < len(parser.body) && parser.body[parser.position] >= '0' && parser.body[parser.position] <= '9' {
			parser.position++
		}
	}
	if parser.consumeByte('.') {
		fractionStart := parser.position
		for parser.position < len(parser.body) && parser.body[parser.position] >= '0' && parser.body[parser.position] <= '9' {
			parser.position++
		}
		if parser.position == fractionStart {
			return errInstructionAuditInvalidJSON
		}
	}
	if parser.position < len(parser.body) && (parser.body[parser.position] == 'e' || parser.body[parser.position] == 'E') {
		parser.position++
		if parser.position < len(parser.body) && (parser.body[parser.position] == '+' || parser.body[parser.position] == '-') {
			parser.position++
		}
		exponentStart := parser.position
		for parser.position < len(parser.body) && parser.body[parser.position] >= '0' && parser.body[parser.position] <= '9' {
			parser.position++
		}
		if parser.position == exponentStart {
			return errInstructionAuditInvalidJSON
		}
	}
	if parser.position == start || !parser.atValueBoundary() {
		return errInstructionAuditInvalidJSON
	}
	return parser.checkDeadline()
}

func (parser *strictInstructionJSONParser) beginValue(depth int) error {
	if depth > parser.limits.MaxDepth {
		return fmt.Errorf("%w: nesting too deep", errInstructionAuditInvalidJSON)
	}
	parser.values++
	if parser.values > parser.limits.MaxJSONValues {
		return errInstructionAuditComplexityExceeded
	}
	return parser.checkDeadline()
}

func (parser *strictInstructionJSONParser) skipWhitespace() {
	for parser.position < len(parser.body) {
		switch parser.body[parser.position] {
		case ' ', '\t', '\r', '\n':
			parser.position++
		default:
			return
		}
	}
}

func (parser *strictInstructionJSONParser) peekByte() byte {
	if parser.position >= len(parser.body) {
		return 0
	}
	return parser.body[parser.position]
}

func (parser *strictInstructionJSONParser) consumeByte(expected byte) bool {
	if parser.position >= len(parser.body) || parser.body[parser.position] != expected {
		return false
	}
	parser.position++
	return true
}

func (parser *strictInstructionJSONParser) atValueBoundary() bool {
	if parser.position >= len(parser.body) {
		return true
	}
	switch parser.body[parser.position] {
	case ' ', '\t', '\r', '\n', ',', '}', ']':
		return true
	default:
		return false
	}
}

func (parser *strictInstructionJSONParser) checkDeadline() error {
	if parser.position < parser.nextClockCheck && parser.values%256 != 0 {
		return nil
	}
	parser.nextClockCheck = parser.position + instructionAuditClockInterval
	if parser.limits.now().After(parser.deadline) {
		return errInstructionAuditParseTimeout
	}
	return nil
}
