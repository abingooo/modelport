package securityaudit

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testReviewerModel = "test-reviewer-model"

func TestParseStructuredReviewerStrictAndPolicy(t *testing.T) {
	tests := []struct {
		name, output string
		enabled      []string
		decision     EventDecision
		action       Action
		wantErr      bool
	}{
		{"safe", `{"safety":"safe","categories":[]}`, AllScannerIDs, EventPass, ActionAllow, false},
		{"controversial", `{"safety":"controversial","categories":["violent"]}`, AllScannerIDs, EventFlag, ActionWarn, false},
		{"controversial pii escalates", `{"safety":"controversial","categories":["pii"]}`, AllScannerIDs, EventCritical, ActionBlock, false},
		{"unsafe", `{"safety":"unsafe","categories":["jailbreak"]}`, AllScannerIDs, EventCritical, ActionBlock, false},
		{"disabled unsafe warns", `{"safety":"unsafe","categories":["violent"]}`, []string{"pii"}, EventFlag, ActionWarn, false},
		{"extra explanation", `{"safety":"safe","categories":[]} trailing`, AllScannerIDs, "", "", true},
		{"duplicate", `{"safety":"safe","safety":"safe","categories":[]}`, AllScannerIDs, "", "", true},
		{"extra key", `{"safety":"safe","categories":[],"reason":"ok"}`, AllScannerIDs, "", "", true},
		{"missing categories", `{"safety":"safe"}`, AllScannerIDs, "", "", true},
		{"unknown safety", `{"safety":"maybe","categories":[]}`, AllScannerIDs, "", "", true},
		{"unknown category", `{"safety":"unsafe","categories":["future_risk"]}`, AllScannerIDs, "", "", true},
		{"safe with category", `{"safety":"safe","categories":["pii"]}`, AllScannerIDs, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseStructuredReviewer(tt.output, tt.enabled)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.decision, result.Decision)
			require.Equal(t, tt.action, result.Action)
		})
	}
}

func TestStructuredReviewerOfficialCategories(t *testing.T) {
	payload, err := json.Marshal(map[string]any{"safety": "unsafe", "categories": AllScannerIDs})
	require.NoError(t, err)
	result, err := ParseStructuredReviewer(string(payload), AllScannerIDs)
	require.NoError(t, err)
	require.Equal(t, AllScannerIDs, result.MatchedScanners)
	require.Empty(t, result.UnknownCategories)
	require.Equal(t, "prompt_audit_structured", result.PolicyID)
	require.Equal(t, 1, result.PolicyVersion)

	aliases := map[string]string{
		"violence": "violent", "non_violent_illegal_acts": "non_violent_illegal_acts",
		"sexual": "sexual_content_or_sexual_acts", "personal identifiable information": "pii",
		"suicide/self harm": "suicide_and_self_harm", "unethical": "unethical_acts",
		"political": "politically_sensitive_topics", "copyright": "copyright_violation",
		"prompt injection": "jailbreak",
	}
	for alias, canonical := range aliases {
		require.Equal(t, canonical, NormalizeCategory(alias), alias)
	}

}

func TestExtractReviewerContentRequiresNormalFinishAndFinalContent(t *testing.T) {
	content, err := extractReviewerContent([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"{\"safety\":\"safe\",\"categories\":[]}","reasoning_content":"ignore"}}]}`))
	require.NoError(t, err)
	require.JSONEq(t, `{"safety":"safe","categories":[]}`, content)
	for _, body := range []string{
		`{"choices":[{"finish_reason":"length","message":{"content":"{}"}}]}`,
		`{"choices":[{"finish_reason":"content_filter","message":{"content":"{}"}}]}`,
		`{"choices":[{"finish_reason":"stop","message":{"content":"","reasoning_content":"{\"safety\":\"safe\",\"categories\":[]}"}}]}`,
	} {
		_, err := extractReviewerContent([]byte(body))
		require.Error(t, err)
	}
}

func TestExtractOpenAIContentSupportsStringAndTextBlocks(t *testing.T) {
	content, err := extractOpenAIContent([]byte(`{"choices":[{"message":{"content":"first line\nsecond line"}}]}`))
	require.NoError(t, err)
	require.Equal(t, "first line\nsecond line", content)
	content, err = extractOpenAIContent([]byte(`{"choices":[{"message":{"content":[{"type":"text","text":"first"},{"type":"text","text":"second"}]}}]}`))
	require.NoError(t, err)
	require.Equal(t, "first\nsecond", content)
	for _, body := range []string{`{}`, `{"choices":[]}`, `{"choices":[{"message":{"content":null}}]}`} {
		_, err := extractOpenAIContent([]byte(body))
		require.Error(t, err)
	}
}

func TestAggregateRequiresEveryResult(t *testing.T) {
	_, err := AggregateResults([]*NormalizedResult{{Decision: EventPass, Action: ActionAllow}, nil}, 0)
	require.Error(t, err)
	result, err := AggregateResults([]*NormalizedResult{
		{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, Categories: []string{"pii"}},
		{Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock, Categories: []string{"jailbreak"}},
	}, 0)
	require.NoError(t, err)
	require.Equal(t, EventCritical, result.Decision)
	require.Equal(t, ActionBlock, result.Action)
	require.Equal(t, []string{"pii", "jailbreak"}, result.Categories)
}

func TestAggregatePreservesSafetyWhenEveryChunkPasses(t *testing.T) {
	result, err := AggregateResults([]*NormalizedResult{
		{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, Safety: "Safe", GuardEndpointID: "safe-node"},
		{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, Safety: "Safe", GuardEndpointID: "safe-node"},
	}, 0)
	require.NoError(t, err)
	require.Equal(t, EventPass, result.Decision)
	require.Equal(t, ActionAllow, result.Action)
	require.Equal(t, "Safe", result.Safety)
}

func TestAggregateDeduplicatesFactsAndUsesMostSevereEndpointMetadata(t *testing.T) {
	result, err := AggregateResults([]*NormalizedResult{
		{Decision: EventPass, RiskLevel: RiskLow, Action: ActionAllow, Safety: "Safe", Categories: []string{"pii"}, MatchedScanners: []string{"pii"}, ScannerScores: map[string]float64{"pii": 0}, ScannerEvidence: map[string]string{"pii": "first"}, GuardEndpointID: "safe-node", ScannerVersion: "safe-version", PolicyID: "priority", PolicyVersion: 1},
		{Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock, Safety: "Unsafe", Categories: []string{"pii", "jailbreak"}, MatchedScanners: []string{"pii", "jailbreak"}, ScannerScores: map[string]float64{"pii": 1, "jailbreak": 1}, ScannerEvidence: map[string]string{"pii": "second", "jailbreak": "blocked"}, GuardEndpointID: "block-node", ScannerVersion: "block-version", PolicyID: "priority", PolicyVersion: 2},
	}, 7*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, []string{"pii", "jailbreak"}, result.Categories)
	require.Equal(t, []string{"pii", "jailbreak"}, result.MatchedScanners)
	require.Equal(t, "first", result.ScannerEvidence["pii"], "evidence is deterministically first-seen")
	require.Equal(t, "block-node", result.GuardEndpointID)
	require.Equal(t, "block-version", result.ScannerVersion)
	require.Equal(t, 2, result.PolicyVersion)
	require.Equal(t, 7, result.LatencyMS)
}

func TestIssueSummariesAreDeterministicRedactedDerivedDTOs(t *testing.T) {
	const canary = "PROMPT_CANARY_EVIDENCE_SECRET"
	result := NormalizedResult{
		Decision: EventCritical, RiskLevel: RiskCritical, Action: ActionBlock,
		Categories: []string{"jailbreak", "pii"}, MatchedScanners: []string{"pii"},
		ScannerScores: map[string]float64{"pii": 1}, ScannerEvidence: map[string]string{"pii": canary},
		UnknownCategories: []string{"unknown:0123456789abcdef"},
	}
	summaries := BuildIssueSummaries(result)
	require.Len(t, summaries, 3, "known categories are not hidden merely because policy disabled one")
	raw, err := json.Marshal(summaries)
	require.NoError(t, err)
	require.NotContains(t, string(raw), canary)
	for _, summary := range summaries {
		require.NotEmpty(t, summary.Title)
		require.NotEmpty(t, summary.Description)
		require.NotEmpty(t, summary.Code)
		require.NotEmpty(t, summary.EvidenceHash)
	}
}
