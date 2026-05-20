package shell_test

import (
	"errors"
	"testing"

	"github.com/arafa-dev/ccx/internal/contracts"
	"github.com/arafa-dev/ccx/internal/shell"
)

func TestEmitUseScript_UnknownShell(t *testing.T) {
	e := shell.New()

	// ShellUnknown is the zero value; it is an explicit "unknown" sentinel.
	_, err := e.EmitUseScript(contracts.Profile{Name: "work", ConfigDir: "/x"}, contracts.ShellUnknown)
	if err == nil {
		t.Fatal("expected error for ShellUnknown, got nil")
	}
	if !errors.Is(err, contracts.ErrUnknownShell) {
		t.Errorf("errors.Is(err, ErrUnknownShell) = false; err = %v", err)
	}

	// Out-of-range shell value also routes to the unknown branch.
	_, err = e.EmitUseScript(contracts.Profile{Name: "work", ConfigDir: "/x"}, contracts.Shell(999))
	if err == nil {
		t.Fatal("expected error for Shell(999), got nil")
	}
	if !errors.Is(err, contracts.ErrUnknownShell) {
		t.Errorf("errors.Is(err, ErrUnknownShell) = false; err = %v", err)
	}
}

func TestEmitInitScript_UnknownShell(t *testing.T) {
	e := shell.New()

	_, err := e.EmitInitScript(contracts.ShellUnknown)
	if err == nil {
		t.Fatal("expected error for ShellUnknown, got nil")
	}
	if !errors.Is(err, contracts.ErrUnknownShell) {
		t.Errorf("errors.Is(err, ErrUnknownShell) = false; err = %v", err)
	}

	_, err = e.EmitInitScript(contracts.Shell(999))
	if err == nil {
		t.Fatal("expected error for Shell(999), got nil")
	}
	if !errors.Is(err, contracts.ErrUnknownShell) {
		t.Errorf("errors.Is(err, ErrUnknownShell) = false; err = %v", err)
	}
}

// The wrapped error must include context (the word "use" or "init" plus,
// where available, the shell name). This protects against a future refactor
// that silently drops context.
func TestEmitUseScript_UnknownShell_WrappedMessage(t *testing.T) {
	e := shell.New()
	_, err := e.EmitUseScript(contracts.Profile{Name: "x", ConfigDir: "/x"}, contracts.ShellUnknown)
	if err == nil {
		t.Fatal("expected error")
	}
	if msg := err.Error(); msg == contracts.ErrUnknownShell.Error() {
		t.Errorf("error is not wrapped with context; got bare sentinel message: %q", msg)
	}
}
