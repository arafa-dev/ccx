package shell

import "strings"

// escapePosixSingle wraps s in single quotes for POSIX shells (sh/bash/zsh/fish).
// Inside single quotes nothing is interpreted, including backslashes, so the
// only character that needs special handling is the single quote itself.
// We close the quote, insert a quoted literal single quote, and reopen:
//
//	it's   ->   'it'"'"'s'
//
// This is the canonical POSIX-portable form used by tools like git, bash's
// printf %q (close enough), and Python's shlex.quote.
func escapePosixSingle(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// escapePowerShellSingle wraps s in single quotes for PowerShell. PowerShell
// uses doubled single quotes (”) to represent a literal single quote inside
// a single-quoted string. Inside single quotes nothing else is interpreted.
func escapePowerShellSingle(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
