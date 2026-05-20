# ccx Engineering Conventions

This document is the source of truth for cross-package conventions. Phase 1
worktrees follow these rules without negotiation. Changes require a PR against
main, not a feature-branch edit.

## 1. Go style

- Format with `gofumpt` (stricter than `gofmt`). Pre-commit hook enforces.
- Lint with `golangci-lint`. Config in `.golangci.yml`. CI gates on it.
- Tabs for indentation in Go files. Spaces in YAML, SQL, Markdown.
- All exported types and functions documented with a Go doc comment beginning
  with the name of the symbol.
- Files end with a trailing newline.

## 2. Error handling

- Always wrap with context: `fmt.Errorf("loading profile %q: %w", name, err)`.
- Define shared sentinel errors in `internal/contracts/errors.go` when multiple
  packages need to detect them. Package-local sentinel errors may live in their
  package when they are specific to that package API. Use `errors.Is` to detect.
- Do not return raw `errors.New(...)` from a public function for a known case —
  add a sentinel.
- Do not log AND return; one or the other (callers log at the boundary).

## 3. Logging

- Use `log/slog` (Go 1.21+ stdlib). No third-party loggers.
- Default level: INFO. `--verbose` raises to DEBUG.
- Default handler: text on stderr. `--log-format=json` switches to JSON.
- Standard field names: `profile`, `path`, `count`, `duration`, `err`.

## 4. CLI exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | User error (bad input, missing profile, etc.) |
| 2 | Internal error |
| 64 | EX_USAGE: command-line usage error |
| 70 | EX_SOFTWARE: internal software error |
| 73 | EX_CANTCREAT: can't create file |
| 74 | EX_IOERR: input/output error |

Use the higher (sysexits) codes only when they communicate something more
specific than the generic 1/2.

## 5. Context propagation

Every public function that does I/O or could block takes a `context.Context`
as its first parameter, named `ctx`. Callers must respect cancellation.

## 6. Package boundaries

- `internal/contracts/` is the only shared package. Every other `internal/*`
  package imports from `contracts` and from stdlib, plus its own declared
  dependencies. No cross-imports between sibling `internal/*` packages
  during Phase 1.
- Phase 2 wires packages together in `internal/cli/`, `internal/server/`,
  `internal/tui/`, `internal/doctor/`, and `cmd/ccx/`.

## 7. Testing

- Table-driven tests preferred for pure functions.
- One test file per source file: `foo.go` ↔ `foo_test.go`.
- Use `_test` package suffix (`package contracts_test`) for black-box tests.
- Use the same-package suffix (`package contracts`) only when testing
  unexported helpers.
- Use `testdata/` for fixtures. Test files inside `testdata/` are never
  compiled (the dir name is ignored by Go).
- Run with `-race -count=1` in CI (no cached results).

## 8. Commit messages

Conventional commits: `type(scope): subject`.

Common types: `feat`, `fix`, `chore`, `docs`, `test`, `refactor`, `perf`,
`build`, `ci`. Scope is the package name (e.g., `feat(contracts):`,
`fix(storage):`).

One logical change per commit. Do not batch unrelated changes.

## 9. Branch and worktree naming

- `feat/<package>` for Phase 1 packages (e.g., `feat/profile`, `feat/scanner`)
- `chore/<topic>` for non-feature work
- Worktree directories: `../ccx-<package>` (sibling of repo)

## 10. PR requirements

- One package per PR
- All tests pass in the worktree before opening
- PR description includes: "Did this work require any contract change? If yes,
  link the amendment PR."
- At least one CI run green before merge

## 11. Security

- Never log credential contents or `.credentials.json` paths from non-default
  config dirs unless explicitly requested by the user with `--debug`.
- Dashboard HTTP server binds to 127.0.0.1 only. Never 0.0.0.0.
- No outbound network calls from the binary except: (a) update checks (opt-in,
  later release), (b) `go mod download` at build time. The runtime is
  offline-by-default.

## 12. Pricing data

- The embedded `pricing/models.yaml` is the v0.1 baseline.
- All currency displays are labeled "Estimated USD" — Anthropic rates can
  change without notice.
- User overrides via `~/.ccx/pricing.yaml` are respected if present and valid.
