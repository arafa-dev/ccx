package plandetect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type claudeConfig struct {
	OAuthAccount struct {
		OrganizationType      string `json:"organizationType"`
		OrganizationRateLimit string `json:"organizationRateLimitTier"`
	} `json:"oauthAccount"`
}

// Detect reads <configDir>/.claude.json and returns the ccx plan tier ("pro")
// inferred from oauthAccount.organizationType. ok is false when the file is
// missing/unreadable or the type is unrecognized, in which case the caller
// should fall back to manually configured limits.
func Detect(configDir string) (tier string, ok bool) {
	if configDir == "" {
		return "", false
	}
	// #nosec G304 -- configDir is a local Claude profile path selected by the user.
	b, err := os.ReadFile(filepath.Join(configDir, ".claude.json"))
	if err != nil {
		return "", false
	}
	var cfg claudeConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return "", false
	}
	return mapOrgType(cfg.OAuthAccount.OrganizationType, cfg.OAuthAccount.OrganizationRateLimit)
}

func mapOrgType(orgType, _ string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(orgType)) {
	case "claude_pro":
		return "pro", true
	default:
		return "", false
	}
}
