// Package shell emits shell-specific snippets for `ccx use` (eval-style) and
// `ccx init` (one-time rc-file paste). It supports zsh, bash, fish, and
// PowerShell, and properly escapes profile names and config directory paths.
//
// This package depends only on internal/contracts and the standard library.
package shell
