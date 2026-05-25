//go:build windows

package run

import "os"

func signalTerminateProcess(process *os.Process) error {
	return process.Kill()
}
