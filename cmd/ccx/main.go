// Package main is the ccx CLI entry point.
//
// THIS FILE IS A TEMPORARY STUB.
//
// Plan A8 (distribution) needs a buildable main package so goreleaser can
// cross-compile snapshot releases before Phase 2 wires up the real cobra
// command tree. Phase 2 P2 replaces this file in full -- when that happens,
// keep the three ldflags-injected variables (version, commit, date) so the
// release pipeline keeps working.
package main

import (
	"fmt"
	"os"
	"runtime"
)

// These are populated by `-ldflags "-X main.version=... -X main.commit=... -X main.date=..."`
// at release time via .goreleaser.yaml. For local `go build`, they remain "dev".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("ccx %s (commit %s, built %s, %s/%s)\n",
			version, commit, date, runtime.GOOS, runtime.GOARCH)
		return
	}
	fmt.Fprintln(os.Stderr, "ccx: pre-release stub. Real CLI lands in Phase 2.")
	fmt.Fprintln(os.Stderr, "Run `ccx version` to print build info.")
	os.Exit(0)
}
