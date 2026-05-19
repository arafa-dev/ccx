# Contributing to ccx

Thanks for your interest! ccx is in active early development. The contribution
flow is:

1. **Read** [`docs/conventions.md`](docs/conventions.md) for style and workflow
   rules. They are not negotiable.
2. **Open an issue** before starting non-trivial work. Tag with the relevant
   `area/<package>` label.
3. **Fork + branch** off `main`. Branch naming: `feat/<topic>` or `fix/<topic>`.
4. **Write tests first** for any new behavior.
5. **Run** `make ci` locally before pushing.
6. **Open a PR.** The PR template asks two questions; please answer both.

## Setting up a dev environment

```bash
git clone https://github.com/arafa-dev/ccx.git
cd ccx
go install mvdan.cc/gofumpt@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
# (optional) lefthook install
make test
```

## Phase 1 worktrees

If you're picking up a specific Phase 1 package (e.g., `internal/scanner`):

```bash
git worktree add ../ccx-scanner -b feat/scanner main
cd ../ccx-scanner
```

Work in your own worktree. Do not touch files outside your assigned package.
If you discover that a shared contract (`internal/contracts/`, `api/openapi.yaml`,
`docs/conventions.md`, `internal/storage/schema.sql`) needs to change, open an
issue and pause your worktree until a contract-amendment PR is merged to main.
