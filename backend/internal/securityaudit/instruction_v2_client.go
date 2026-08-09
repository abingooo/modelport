package securityaudit

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var instructionV2ProfileKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

func compileInstructionV2ClientProfile(profile InstructionV2ClientProfile) (instructionV2ClientRuntime, error) {
	profile.ProfileKey = strings.ToLower(strings.TrimSpace(profile.ProfileKey))
	profile.Name = strings.TrimSpace(profile.Name)
	profile.Description = strings.TrimSpace(profile.Description)
	if !instructionV2ProfileKeyPattern.MatchString(profile.ProfileKey) || profile.Name == "" {
		return instructionV2ClientRuntime{}, errors.New("invalid instruction audit client profile")
	}
	if len(profile.Matchers) > 32 {
		return instructionV2ClientRuntime{}, errors.New("instruction audit client profile has too many matchers")
	}
	runtime := instructionV2ClientRuntime{profile: profile, matchers: make([]instructionV2CompiledMatcher, 0, len(profile.Matchers))}
	for _, matcher := range profile.Matchers {
		compiled, err := compileInstructionV2ClientMatcher(matcher)
		if err != nil {
			return instructionV2ClientRuntime{}, err
		}
		runtime.matchers = append(runtime.matchers, compiled)
	}
	if profile.ImmutableInternal && (profile.ProfileKey != InstructionClientModelPortInternal || len(profile.Matchers) != 0 || !profile.Enabled || !profile.BuiltIn) {
		return instructionV2ClientRuntime{}, errors.New("invalid immutable internal client profile")
	}
	return runtime, nil
}

func compileInstructionV2ClientMatcher(matcher InstructionV2ClientMatcher) (instructionV2CompiledMatcher, error) {
	matcher.Type = strings.ToLower(strings.TrimSpace(matcher.Type))
	matcher.Value = strings.TrimSpace(matcher.Value)
	if matcher.Value == "" || len(matcher.Value) > 512 {
		return instructionV2CompiledMatcher{}, errors.New("instruction audit client matcher is empty or too long")
	}
	compiled := instructionV2CompiledMatcher{
		matcherType: matcher.Type, value: matcher.Value, caseSensitive: matcher.CaseSensitive,
	}
	switch matcher.Type {
	case "prefix":
		return compiled, nil
	case "regex":
		pattern := matcher.Value
		if !matcher.CaseSensitive {
			pattern = "(?i:" + pattern + ")"
		}
		regex, err := regexp.Compile(pattern)
		if err != nil {
			return instructionV2CompiledMatcher{}, fmt.Errorf("invalid RE2 client matcher: %w", err)
		}
		compiled.regex = regex
		return compiled, nil
	default:
		return instructionV2CompiledMatcher{}, errors.New("instruction audit client matcher type must be prefix or regex")
	}
}

func normalizeInstructionV2ClientProfiles(profiles []InstructionV2ClientProfile) ([]instructionV2ClientRuntime, map[string]instructionV2ClientRuntime, error) {
	runtimes := make([]instructionV2ClientRuntime, 0, len(profiles))
	byKey := make(map[string]instructionV2ClientRuntime, len(profiles))
	for _, profile := range profiles {
		runtime, err := compileInstructionV2ClientProfile(profile)
		if err != nil {
			return nil, nil, err
		}
		if _, duplicate := byKey[runtime.profile.ProfileKey]; duplicate {
			return nil, nil, errors.New("duplicate instruction audit client profile")
		}
		byKey[runtime.profile.ProfileKey] = runtime
		if runtime.profile.Enabled {
			runtimes = append(runtimes, runtime)
		}
	}
	for _, required := range []string{InstructionClientModelPortInternal, InstructionClientOther, InstructionClientUnknown} {
		if _, ok := byKey[required]; !ok {
			return nil, nil, fmt.Errorf("required instruction audit client profile %q is missing", required)
		}
	}
	sort.SliceStable(runtimes, func(left, right int) bool {
		if runtimes[left].profile.Priority != runtimes[right].profile.Priority {
			return runtimes[left].profile.Priority < runtimes[right].profile.Priority
		}
		return runtimes[left].profile.ID < runtimes[right].profile.ID
	})
	return runtimes, byKey, nil
}

func classifyInstructionV2Client(snapshot *instructionV2Snapshot, userAgent string, trustedInternal bool) instructionV2ClientRuntime {
	if snapshot == nil {
		return instructionV2FallbackClient(InstructionClientUnknown)
	}
	if trustedInternal {
		if profile, ok := snapshot.ProfilesByKey[InstructionClientModelPortInternal]; ok {
			return profile
		}
		return instructionV2FallbackClient(InstructionClientModelPortInternal)
	}
	userAgent = strings.TrimSpace(userAgent)
	if !validInstructionUserAgent(userAgent) {
		if profile, ok := snapshot.ProfilesByKey[InstructionClientUnknown]; ok {
			return profile
		}
		return instructionV2FallbackClient(InstructionClientUnknown)
	}
	for _, profile := range snapshot.Profiles {
		switch profile.profile.ProfileKey {
		case InstructionClientModelPortInternal, InstructionClientOther, InstructionClientUnknown:
			continue
		}
		if instructionV2ClientProfileMatches(profile, userAgent) {
			return profile
		}
	}
	if profile, ok := snapshot.ProfilesByKey[InstructionClientOther]; ok {
		return profile
	}
	return instructionV2FallbackClient(InstructionClientOther)
}

func instructionV2ClientProfileMatches(profile instructionV2ClientRuntime, userAgent string) bool {
	for _, matcher := range profile.matchers {
		switch matcher.matcherType {
		case "prefix":
			if matcher.caseSensitive && strings.HasPrefix(userAgent, matcher.value) {
				return true
			}
			if !matcher.caseSensitive && len(userAgent) >= len(matcher.value) && strings.EqualFold(userAgent[:len(matcher.value)], matcher.value) {
				return true
			}
		case "regex":
			if matcher.regex != nil && matcher.regex.MatchString(userAgent) {
				return true
			}
		}
	}
	return false
}

func instructionV2FallbackClient(key string) instructionV2ClientRuntime {
	name := "Unknown"
	if key == InstructionClientOther {
		name = "Other"
	} else if key == InstructionClientModelPortInternal {
		name = "ModelPort Internal"
	}
	return instructionV2ClientRuntime{profile: InstructionV2ClientProfile{ProfileKey: key, Name: name, Enabled: true}}
}

func normalizeInstructionV2ClientProfileRequest(request SaveInstructionV2ClientProfileRequest) (SaveInstructionV2ClientProfileRequest, error) {
	request.ProfileKey = strings.ToLower(strings.TrimSpace(request.ProfileKey))
	request.Name = strings.TrimSpace(request.Name)
	request.Description = strings.TrimSpace(request.Description)
	if !instructionV2ProfileKeyPattern.MatchString(request.ProfileKey) || request.Name == "" || len(request.Name) > 120 || len(request.Description) > 500 {
		return request, errors.New("invalid instruction audit client profile")
	}
	profile := InstructionV2ClientProfile{
		ProfileKey: request.ProfileKey, Name: request.Name, Description: request.Description,
		Matchers: request.Matchers, Priority: request.Priority, Enabled: request.Enabled,
	}
	if _, err := compileInstructionV2ClientProfile(profile); err != nil {
		return request, err
	}
	if request.ProfileKey == InstructionClientModelPortInternal || request.ProfileKey == InstructionClientOther || request.ProfileKey == InstructionClientUnknown {
		return request, errors.New("reserved instruction audit client profile key")
	}
	return request, nil
}
