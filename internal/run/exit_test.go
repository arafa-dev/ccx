package run_test

import (
	"errors"
	"testing"

	"github.com/arafa-dev/ccx/internal/run"
)

func TestExitCodeErrorMessage(t *testing.T) {
	e := run.ExitCodeError{Code: 7}
	if e.Error() != "exit 7" {
		t.Errorf("Error() = %q, want \"exit 7\"", e.Error())
	}
	if e.ExitCode() != 7 {
		t.Errorf("ExitCode() = %d, want 7", e.ExitCode())
	}
}

func TestExitCodeErrorAs(t *testing.T) {
	var coded run.ExitCodeError
	err := run.ExitCodeError{Code: 3}
	if !errors.As(err, &coded) {
		t.Fatal("errors.As did not match")
	}
	if coded.Code != 3 {
		t.Errorf("Code = %d, want 3", coded.Code)
	}
}
