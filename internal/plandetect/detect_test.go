package plandetect

import (
	"os"
	"path/filepath"
	"testing"
)

func writeClaudeJSON(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".claude.json"), []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestDetectPro(t *testing.T) {
	dir := t.TempDir()
	writeClaudeJSON(t, dir, `{"oauthAccount":{"organizationType":"claude_pro","organizationRateLimitTier":"default_claude_ai"}}`)
	tier, ok := Detect(dir)
	if !ok || tier != "pro" {
		t.Fatalf("want pro,true; got %q,%v", tier, ok)
	}
}

func TestDetectUnknownFallsBack(t *testing.T) {
	dir := t.TempDir()
	writeClaudeJSON(t, dir, `{"oauthAccount":{"organizationType":"something_new"}}`)
	if tier, ok := Detect(dir); ok {
		t.Fatalf("unknown type must return ok=false; got %q,%v", tier, ok)
	}
}

func TestDetectMissingFile(t *testing.T) {
	if tier, ok := Detect(t.TempDir()); ok {
		t.Fatalf("missing file must return ok=false; got %q,%v", tier, ok)
	}
}
