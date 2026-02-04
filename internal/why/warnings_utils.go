package why

func dedupeStringsPreserveOrder(in []string) []string {
	if len(in) < 2 {
		return in
	}

	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
