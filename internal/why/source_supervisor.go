package why

import (
	"strings"
)

var knownSupervisors = map[string]string{
	"pm2":          "pm2",
	"pm2 god":      "pm2",
	"supervisord":  "supervisord",
	"gunicorn":     "gunicorn",
	"uwsgi":        "uwsgi",
	"s6-supervise": "s6",
	"s6":           "s6",
	"runsv":        "runit",
	"runit":        "runit",
	"openrc":       "openrc",
	"monit":        "monit",
	"circusd":      "circus",
	"circus":       "circus",
	"daemontools":  "daemontools",
	"tini":         "tini",
	"docker-init":  "docker-init",
	// systemd/init are handled separately by ancestry analysis but can be caught here too
}

func detectSupervisor(ancestry []ProcessInfo) *Source {
	for _, p := range ancestry {
		// Normalize: remove spaces, lowercase
		pname := strings.ReplaceAll(strings.ToLower(p.Command), " ", "")
		pcmd := strings.ReplaceAll(strings.ToLower(p.Cmdline), " ", "")

		// PM2 Specific Check
		if strings.Contains(pname, "pm2") || strings.Contains(pcmd, "pm2") {
			return &Source{
				Type:       SourcePM2,
				Name:       "pm2",
				Confidence: 0.9,
			}
		}

		// Exact command match
		if label, ok := knownSupervisors[strings.ToLower(p.Command)]; ok {
			return &Source{
				Type:       SourceSupervisor,
				Name:       label,
				Confidence: 0.8,
			}
		}

		// Cmdline keyword match
		for sup, label := range knownSupervisors {
			if strings.Contains(strings.ToLower(p.Cmdline), sup) {
				return &Source{
					Type:       SourceSupervisor,
					Name:       label,
					Confidence: 0.7,
				}
			}
		}
	}
	return nil
}
