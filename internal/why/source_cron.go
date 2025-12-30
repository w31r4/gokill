package why

func detectCron(ancestry []ProcessInfo) *Source {
	for _, p := range ancestry {
		if p.Command == "cron" || p.Command == "crond" {
			return &Source{
				Type:       SourceCron,
				Name:       "cron",
				Confidence: 0.6,
			}
		}
	}
	return nil
}
