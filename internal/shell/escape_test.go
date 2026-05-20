package shell

import "testing"

func TestEscapePosixSingle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "''"},
		{"simple", "work", "'work'"},
		{"with space", "my work", "'my work'"},
		{"with path", "/Users/arafa/.claude-profiles/work", "'/Users/arafa/.claude-profiles/work'"},
		{"with single quote", "it's", `'it'"'"'s'`},
		{"only single quote", "'", `''"'"''`},
		{"path with quote and space", "/tmp/it's a dir", `'/tmp/it'"'"'s a dir'`},
		{"double quote untouched", `say "hi"`, `'say "hi"'`},
		{"backslash untouched", `a\b`, `'a\b'`},
		{"dollar untouched", `$HOME`, `'$HOME'`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapePosixSingle(tc.in); got != tc.want {
				t.Errorf("escapePosixSingle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestEscapePowerShellSingle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "''"},
		{"simple", "work", "'work'"},
		{"with space", "my work", "'my work'"},
		{"with path", `C:\Users\arafa\.claude-profiles\work`, `'C:\Users\arafa\.claude-profiles\work'`},
		{"with single quote", "it's", "'it''s'"},
		{"only single quote", "'", "''''"},
		{"path with quote and space", `C:\tmp\it's a dir`, `'C:\tmp\it''s a dir'`},
		{"double quote untouched", `say "hi"`, `'say "hi"'`},
		{"dollar untouched", `$env:HOME`, `'$env:HOME'`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := escapePowerShellSingle(tc.in); got != tc.want {
				t.Errorf("escapePowerShellSingle(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
