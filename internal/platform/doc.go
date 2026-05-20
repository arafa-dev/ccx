// Package platform contains small OS-conditional helpers used by the rest of
// ccx: resolving the default Claude Code config directory, locating the ccx
// state directory, expanding user-supplied paths, detecting the user's shell,
// and figuring out where Claude Code stores credentials per OS.
//
// Implementation files are split via build tags:
//
//	platform.go           common, OS-independent helpers
//	platform_darwin.go    macOS-specific (keychain credentials)
//	platform_linux.go     Linux-specific (file credentials, $SHELL parsing)
//	platform_windows.go   Windows-specific (%USERPROFILE%, PowerShell heuristics)
//
// The public API is identical on every platform. Only the implementation
// switches. Callers do not need to do their own GOOS checks.
package platform
