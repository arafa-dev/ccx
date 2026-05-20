# ccx dashboard

Next.js 15 static-export dashboard for ccx. Embedded by the Go binary at build
time via `//go:embed`.

## Development

```bash
pnpm install
pnpm dev          # http://localhost:3001 - MSW serves mock data
pnpm build        # produces ./out/ for Go embed
pnpm test         # vitest
pnpm e2e          # playwright (run `pnpm e2e:install` once first)
```

## Regenerating API types

`lib/api-types.ts` is generated from `../api/openapi.yaml`. Regenerate after any
contract change:

```bash
pnpm gen:api
```

CI fails if the committed `lib/api-types.ts` is out of date.
