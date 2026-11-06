package core

// Exit codes of omt, from docs/agents.md section 7. They do not change
// within a major version.
const (
	ExitOK      = 0 // the action ran
	ExitError   = 1 // the action failed
	ExitUsage   = 2 // the command line was wrong
	ExitConfirm = 3 // a person must confirm, and none could be asked
	ExitDenied  = 4 // refused, or outside what the tool may touch
)
