package why

import (
	"sort"
	"strings"
)

type envSuspiciousRule struct {
	pattern     string
	match       func(key, pattern string) bool
	warning     string
	includeKeys bool
}

var envVarRules = []envSuspiciousRule{
	{
		pattern: "LD_PRELOAD",
		match:   func(key, pattern string) bool { return key == pattern },
		warning: "Process sets LD_PRELOAD (potential library injection)",
	},
	{
		pattern:     "DYLD_",
		match:       strings.HasPrefix,
		warning:     "Process sets DYLD_* variables (potential library injection)",
		includeKeys: true,
	},
}

func commonWarnings(r *AnalysisResult) []string {
	if r == nil {
		return nil
	}

	var w []string

	if r.ExeDeleted {
		w = append(w, "Process is running from a deleted binary (potential library injection or pending update)")
	}

	if r.Source.Type == SourceUnknown {
		w = append(w, "No known supervisor or service manager detected")
	}

	if isSuspiciousWorkingDir(r.WorkingDir) {
		w = append(w, "Process is running from a suspicious working directory: "+r.WorkingDir)
	}

	return w
}

func isSuspiciousWorkingDir(dir string) bool {
	switch dir {
	case "/", "/tmp", "/var/tmp":
		return dir != ""
	default:
		return false
	}
}

func envSuspiciousWarnings(env []string) []string {
	matched := make([]bool, len(envVarRules))
	matchedKeys := make([]map[string]struct{}, len(envVarRules))

	for i, rule := range envVarRules {
		if rule.includeKeys {
			matchedKeys[i] = map[string]struct{}{}
		}
	}

	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || value == "" {
			continue
		}

		for i, rule := range envVarRules {
			if !rule.match(key, rule.pattern) {
				continue
			}
			matched[i] = true
			if rule.includeKeys {
				matchedKeys[i][key] = struct{}{}
			}
		}
	}

	var warnings []string
	for i, rule := range envVarRules {
		if !matched[i] {
			continue
		}
		if !rule.includeKeys {
			warnings = append(warnings, rule.warning)
			continue
		}

		keys := make([]string, 0, len(matchedKeys[i]))
		for key := range matchedKeys[i] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		warnings = append(warnings, rule.warning+": "+strings.Join(keys, ", "))
	}

	return warnings
}
