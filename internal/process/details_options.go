package process

// DetailsOptions controls which optional sections are collected and rendered in the details view.
//
// The default (zero value) MUST stay lightweight to keep the TUI responsive.
type DetailsOptions struct {
	// Verbose enables deeper (bounded) collection and additional sections.
	Verbose bool

	// ShowEnv enables the Env section in the details view.
	ShowEnv bool

	// RevealEnvSecrets controls whether env values are shown without redaction.
	// It only applies when ShowEnv is true.
	RevealEnvSecrets bool
}
