package platform

import "errors"

// ErrCredentialsInKeychain is returned by CredentialsPath on macOS, where
// Claude Code stores credentials in the system Keychain rather than on disk.
// Callers should detect this with errors.Is and skip file-based credential
// checks.
var ErrCredentialsInKeychain = errors.New("credentials stored in macOS Keychain, no file path")
