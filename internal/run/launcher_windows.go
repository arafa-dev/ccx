//go:build windows

package run

import "os"

func forwardedSignals() []os.Signal {
	return []os.Signal{os.Interrupt}
}

func signaledExitCode(error) (int, bool) {
	return 0, false
}
