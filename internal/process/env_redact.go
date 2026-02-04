package process

import "strings"

var envKeyAllowlistExact = map[string]struct{}{
	"COLORTERM":       {},
	"DISPLAY":         {},
	"EDITOR":          {},
	"HOME":            {},
	"HOST":            {},
	"HOSTNAME":        {},
	"LANG":            {},
	"LC_ALL":          {},
	"LC_CTYPE":        {},
	"LOGNAME":         {},
	"MAIL":            {},
	"OLDPWD":          {},
	"PAGER":           {},
	"PATH":            {},
	"PWD":             {},
	"SHELL":           {},
	"SHLVL":           {},
	"TERM":            {},
	"TMP":             {},
	"TMPDIR":          {},
	"TEMP":            {},
	"TZ":              {},
	"USER":            {},
	"USERNAME":        {},
	"VISUAL":          {},
	"WAYLAND_DISPLAY": {},
	"XAUTHORITY":      {},
	"XDG_RUNTIME_DIR": {},
	"SSH_AUTH_SOCK":   {},
	"SSH_TTY":         {},
	"SSH_CONNECTION":  {},
	"SSH_CLIENT":      {},
}

var envKeyAllowlistPrefixes = []string{
	"LC_",
	"XDG_",
}

var envKeyDenySegments = map[string]struct{}{
	"BEARER":      {},
	"CREDENTIAL":  {},
	"CREDENTIALS": {},
	"COOKIE":      {},
	"KEY":         {},
	"PASS":        {},
	"PASSWD":      {},
	"PASSWORD":    {},
	"PRIVATE":     {},
	"SECRET":      {},
	"SESSION":     {},
	"SIGNATURE":   {},
	"SIGNING":     {},
	"TOKEN":       {},
}

func shouldRedactEnvKey(key string) bool {
	k := strings.TrimSpace(key)
	if k == "" {
		return false
	}

	upper := strings.ToUpper(k)
	if _, ok := envKeyAllowlistExact[upper]; ok {
		return false
	}
	for _, prefix := range envKeyAllowlistPrefixes {
		if strings.HasPrefix(upper, prefix) {
			return false
		}
	}

	segs := splitEnvKeySegments(upper)
	for _, seg := range segs {
		if _, ok := envKeyDenySegments[seg]; ok {
			return true
		}
	}

	// Fallback substring matches for non-delimited keys (best-effort).
	for _, sub := range []string{"TOKEN", "SECRET", "PASSWORD", "PASSWD", "PASS"} {
		if strings.Contains(upper, sub) {
			return true
		}
	}

	return false
}

func splitEnvKeySegments(keyUpper string) []string {
	return strings.FieldsFunc(keyUpper, func(r rune) bool {
		return r == '_' || r == '-'
	})
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	// Fast path: most env entries are small.
	if len(s) <= max {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}
