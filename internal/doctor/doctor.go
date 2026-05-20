// Package doctor implements structured diagnostic checks for `ccx doctor`.
package doctor

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/platform"
)

// Deps holds the doctor's dependencies.
type Deps struct {
	Profiles ProfileLister
}

// ProfileLister is the minimal interface doctor needs.
type ProfileLister interface {
	List(ctx context.Context) ([]contracts.Profile, error)
}

// Doctor runs diagnostic checks.
type Doctor struct {
	deps Deps
}

// New constructs a Doctor.
func New(deps Deps) *Doctor { return &Doctor{deps: deps} }

// Run returns the list of checks.
func (d *Doctor) Run(ctx context.Context) ([]contracts.DoctorCheck, error) {
	checks := make([]contracts.DoctorCheck, 0, 5)
	checks = append(
		checks,
		d.checkClaudeOnPath(),
		d.checkCCXHome(),
		d.checkDefaultConfigDir(),
	)
	checks = append(checks, d.checkRegisteredProfiles(ctx)...)
	checks = append(checks, d.checkShellInit())
	return checks, nil
}

func (d *Doctor) checkClaudeOnPath() contracts.DoctorCheck {
	path, err := exec.LookPath("claude")
	if err != nil {
		return contracts.DoctorCheck{
			Name:        "claude on PATH",
			Status:      "fail",
			Detail:      "claude binary not found in PATH",
			Remediation: "Install Claude Code from https://claude.com/code",
		}
	}
	return contracts.DoctorCheck{Name: "claude on PATH", Status: "ok", Detail: path}
}

func (d *Doctor) checkCCXHome() contracts.DoctorCheck {
	home, err := platform.CCXHome()
	if err != nil {
		return contracts.DoctorCheck{
			Name:        "ccx home directory",
			Status:      "fail",
			Detail:      err.Error(),
			Remediation: "Verify HOME or USERPROFILE is set.",
		}
	}
	return contracts.DoctorCheck{Name: "ccx home directory", Status: "ok", Detail: home}
}

func (d *Doctor) checkDefaultConfigDir() contracts.DoctorCheck {
	cfg, err := platform.DefaultConfigDir()
	if err != nil {
		return contracts.DoctorCheck{
			Name:   "default Claude Code config dir",
			Status: "warn",
			Detail: err.Error(),
		}
	}
	if _, err := os.Stat(cfg); err != nil {
		return contracts.DoctorCheck{
			Name:        "default Claude Code config dir",
			Status:      "warn",
			Detail:      fmt.Sprintf("%s does not exist", cfg),
			Remediation: "Run `claude /login` once, or register a profile with a different path.",
		}
	}
	return contracts.DoctorCheck{Name: "default Claude Code config dir", Status: "ok", Detail: cfg}
}

func (d *Doctor) checkRegisteredProfiles(ctx context.Context) []contracts.DoctorCheck {
	if d.deps.Profiles == nil {
		return nil
	}
	profiles, err := d.deps.Profiles.List(ctx)
	if err != nil {
		return []contracts.DoctorCheck{{
			Name:   "registered profiles",
			Status: "fail",
			Detail: err.Error(),
		}}
	}
	if len(profiles) == 0 {
		return []contracts.DoctorCheck{{
			Name:        "registered profiles",
			Status:      "warn",
			Detail:      "no profiles registered",
			Remediation: "Run `ccx profile add <name> --config-dir <path>` to register your first profile.",
		}}
	}
	out := make([]contracts.DoctorCheck, 0, len(profiles))
	for _, p := range profiles {
		status := "ok"
		detail := p.ConfigDir
		if _, err := os.Stat(p.ConfigDir); err != nil {
			status = "warn"
			detail = fmt.Sprintf("config dir missing: %s", p.ConfigDir)
		}
		out = append(out, contracts.DoctorCheck{
			Name:   "profile: " + p.Name,
			Status: status,
			Detail: detail,
		})
	}
	return out
}

func (d *Doctor) checkShellInit() contracts.DoctorCheck {
	if os.Getenv("CCX_ACTIVE_PROFILE") != "" {
		return contracts.DoctorCheck{
			Name:   "shell integration",
			Status: "ok",
			Detail: "active profile detected from env",
		}
	}
	return contracts.DoctorCheck{
		Name:        "shell integration",
		Status:      "warn",
		Detail:      "CCX_ACTIVE_PROFILE not set",
		Remediation: "Add `eval \"$(ccx init zsh)\"` to your shell rc file.",
	}
}
