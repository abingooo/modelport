package securityaudit

import "strings"

const (
	PromptAuditSourceInstructionV2  = "instruction_audit_v2"
	PromptAuditTriggerNonResponses  = "non_responses_protocol"
	PromptAuditModelContractVersion = 2
)

// PromptAuditRoute is retained for legacy Instruction V2 configuration
// compatibility. The upstream Prompt Audit remains the authoritative runtime
// switch and does not consume this route.
type PromptAuditRoute struct {
	Eligible                     bool
	InstructionConfigUnavailable bool
	AuditSource                  string
	InstructionConfigVersion     int64
	ClientProfileKey             string
	ClientProfileName            string
	TriggerReason                string
	ModelContractVersion         int
}

func isResponsesProtocolFamily(protocol, endpoint string) bool {
	normalized := strings.ToLower(strings.TrimSpace(protocol))
	normalized = strings.NewReplacer("-", "_", ".", "_", "/", "_").Replace(normalized)
	if normalized == "responses" || normalized == "openai_responses" ||
		strings.HasPrefix(normalized, "responses_") || strings.HasPrefix(normalized, "openai_responses_") {
		return true
	}
	path := strings.ToLower(strings.TrimSpace(endpoint))
	if index := strings.IndexAny(path, "?#"); index >= 0 {
		path = path[:index]
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "responses" {
			return true
		}
	}
	return false
}
