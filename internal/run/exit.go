package run

import "fmt"

// ExitCodeError lets CLI RunE, supervisor code, or another ccx caller request
// a specific process exit status. CLI wiring can type-assert on this and
// propagate Code while suppressing the noisy default "Error: ..." stderr line.
//
// B3b supervisor code can use this when claude exits non-zero so the child's
// exit code surfaces all the way out to the shell.
type ExitCodeError struct{ Code int }

// Error returns the process exit status as a compact error message.
func (e ExitCodeError) Error() string { return fmt.Sprintf("exit %d", e.Code) }

// ExitCode returns the process exit status to propagate.
func (e ExitCodeError) ExitCode() int { return e.Code }
