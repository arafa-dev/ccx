# ccx Phase 1 — A7 `web/` (Next.js dashboard) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `web/` package — a Next.js 15 App Router static-exportable dashboard that mocks the `ccx` HTTP API via MSW. The dashboard is independent of the Go backend during Phase 1; it consumes only `api/openapi.yaml` for types. Backend integration happens in Phase 2.

**Architecture:** Single-page scrolling layout per spec section 8.4 — Header, ProfileCards row, time-series chart, TopProjects table, RecentSessions list, Footer. Dark mode default with toggle. API client is a thin `fetch` wrapper typed by `openapi-typescript` against `api/openapi.yaml`. In development, MSW intercepts `fetch` and returns realistic fixtures. Production builds are pure static (`next build` → `web/out/`) embedded by the Go server in Phase 2 via `//go:embed`.

**Tech Stack:**
- Next.js 15 (App Router) + React 19
- TypeScript strict
- Tailwind CSS 4 (PostCSS plugin)
- Recharts for charts
- shadcn/ui primitives (Card, Button, Table, Tabs, DropdownMenu) — copied into `web/components/ui/`
- MSW (Mock Service Worker) for API mocking in dev
- `openapi-typescript` for generating typed API surface from `api/openapi.yaml`
- next-themes for dark/light toggle
- Lucide React for icons
- Vitest + React Testing Library + jsdom for unit tests
- Playwright for one E2E happy-path test
- pnpm as the package manager (not npm)

**Spec reference:** [`docs/superpowers/specs/2026-05-19-ccx-design.md`](../specs/2026-05-19-ccx-design.md) — Section 8 (Dashboard). Layout: section 8.4. Visual style: same. Failure modes: section 8.6.

**Contract source of truth:** [`api/openapi.yaml`](../../../api/openapi.yaml) — produced by Phase 0. Every API client function and MSW handler maps 1:1 to an endpoint defined there.

**Worktree:** `feat/web` off `main` (after Phase 0 has been merged). Create with:
```bash
git worktree add ../ccx-web -b feat/web main
cd ../ccx-web
```

**Exit criteria:**
- `pnpm --filter web build` (or `pnpm build` from within `web/`) produces `web/out/index.html` plus static assets
- `pnpm --filter web dev` serves dashboard at `http://localhost:3001` with MSW returning realistic data for all six layout sections
- All Vitest tests pass: `pnpm --filter web test`
- Playwright E2E passes: `pnpm --filter web e2e`
- TypeScript strict-check passes: `pnpm --filter web typecheck`
- ESLint clean: `pnpm --filter web lint`
- Dashboard loads in under 1s on a production preview (`pnpm --filter web preview`)
- Lighthouse (Chrome devtools, mobile config) reports Performance ≥ 90 and Accessibility ≥ 95 on the static export
- All commits pushed; PR opened against `main`

---

## Pre-flight

Confirm working directory is the `feat/web` worktree, Node.js 20+ is available, `pnpm` is installed, and `api/openapi.yaml` exists.

```bash
pwd                                          # → /Users/arafa/Developer/ccx-web (or wherever the worktree lives)
git status                                   # → On branch feat/web, working tree clean
node --version                               # → v20.x or v22.x
pnpm --version                               # → 9.x
test -f api/openapi.yaml && echo OK          # → OK
```

If `pnpm` is not installed:
```bash
corepack enable
corepack prepare pnpm@latest --activate
```

**Conventions for this plan:**
- Every code file is TypeScript (`.ts` or `.tsx`). No `.js` source files (only `next.config.mjs` for the Next config).
- Indentation: 2 spaces (Prettier default), no tabs. Single quotes for strings.
- One commit per task, conventional commit prefix (`feat(web):`, `chore(web):`, `test(web):`).
- TDD where the test surface is meaningful: write the failing component test before the component. Pure infra tasks (config, scaffolding) commit without TDD.
- Never run tests against the real Go server in this phase — MSW handles all `/api/*` traffic.

---

## Task 1: Scaffold the Next.js 15 project

**Files:**
- Create: `web/` directory tree via `create-next-app`
- Modify: `web/package.json`, `web/next.config.mjs`, `web/tsconfig.json`

- [ ] **Step 1: Run `create-next-app` non-interactively**

From repo root:
```bash
pnpm create next-app@latest web \
  --typescript \
  --tailwind \
  --app \
  --eslint \
  --no-src-dir \
  --import-alias "@/*" \
  --use-pnpm \
  --turbopack \
  --skip-install
```

The flags: `--app` enables App Router; `--no-src-dir` puts `app/` at `web/` root (not `web/src/`); `--import-alias "@/*"` maps `@/foo` → `web/foo`; `--turbopack` makes dev startup snappier; `--skip-install` lets us tweak `package.json` before installing.

If the prompt still asks anything interactively, accept defaults.

- [ ] **Step 2: Replace `web/next.config.mjs` with the static-export config**

Open `web/next.config.mjs`. Replace its contents with:

```js
/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'export',
  images: { unoptimized: true },
  trailingSlash: false,
  reactStrictMode: true,
  // The Go server (Phase 2) will serve these assets from /. The export needs
  // assetPrefix '' so URLs are relative.
  assetPrefix: '',
  // Disable Next.js telemetry in CI and dev builds for ccx.
  productionBrowserSourceMaps: false,
};

export default nextConfig;
```

- [ ] **Step 3: Pin Next + React versions in `web/package.json`**

Open `web/package.json`. Set the following:

```json
{
  "name": "@ccx/web",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "scripts": {
    "dev": "next dev -p 3001 --turbopack",
    "build": "next build",
    "start": "next start -p 3001",
    "preview": "pnpm build && npx serve out -p 3002",
    "lint": "next lint",
    "typecheck": "tsc --noEmit",
    "test": "vitest run",
    "test:watch": "vitest",
    "e2e": "playwright test",
    "e2e:install": "playwright install --with-deps chromium",
    "gen:api": "openapi-typescript ../api/openapi.yaml -o ./lib/api-types.ts"
  },
  "dependencies": {
    "next": "15.0.3",
    "react": "19.0.0",
    "react-dom": "19.0.0",
    "recharts": "2.13.3",
    "next-themes": "0.4.3",
    "lucide-react": "0.460.0",
    "class-variance-authority": "0.7.0",
    "clsx": "2.1.1",
    "tailwind-merge": "2.5.4"
  },
  "devDependencies": {
    "@playwright/test": "1.48.2",
    "@testing-library/dom": "10.4.0",
    "@testing-library/jest-dom": "6.6.3",
    "@testing-library/react": "16.0.1",
    "@testing-library/user-event": "14.5.2",
    "@types/node": "22.9.0",
    "@types/react": "19.0.0",
    "@types/react-dom": "19.0.0",
    "@vitejs/plugin-react": "4.3.3",
    "autoprefixer": "10.4.20",
    "eslint": "9.14.0",
    "eslint-config-next": "15.0.3",
    "jsdom": "25.0.1",
    "msw": "2.6.4",
    "openapi-typescript": "7.4.3",
    "postcss": "8.4.49",
    "tailwindcss": "3.4.14",
    "typescript": "5.6.3",
    "vitest": "2.1.5"
  }
}
```

Note: Tailwind 3 is pinned (rather than 4) for production stability and shadcn compatibility. Update if/when shadcn officially supports Tailwind 4 across all primitives.

- [ ] **Step 4: Install dependencies**

```bash
cd web && pnpm install
```

Expected: clean install, no peer-dep warnings that block compilation.

- [ ] **Step 5: Update `tsconfig.json` for strict mode**

Open `web/tsconfig.json`. Ensure `"strict": true` is in `compilerOptions` (create-next-app sets this by default). Add `"noUncheckedIndexedAccess": true`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["dom", "dom.iterable", "esnext"],
    "allowJs": false,
    "skipLibCheck": true,
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "noEmit": true,
    "esModuleInterop": true,
    "module": "esnext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,
    "isolatedModules": true,
    "jsx": "preserve",
    "incremental": true,
    "plugins": [{ "name": "next" }],
    "paths": { "@/*": ["./*"] }
  },
  "include": ["next-env.d.ts", "**/*.ts", "**/*.tsx", ".next/types/**/*.ts"],
  "exclude": ["node_modules", "out", ".next"]
}
```

- [ ] **Step 6: Verify the scaffold builds**

```bash
pnpm build
```

Expected: build succeeds, produces `web/out/index.html`, exits 0.

- [ ] **Step 7: Commit**

From repo root:
```bash
git add web/ .gitignore
git commit -m "feat(web): scaffold Next.js 15 static-export dashboard"
```

If `web/node_modules/`, `web/.next/`, `web/out/` are not yet ignored by repo `.gitignore`, confirm they are (Phase 0 added them). If not, add them and amend the commit.

---

## Task 2: Generate API types from openapi.yaml

**Files:**
- Create: `web/lib/api-types.ts` (generated)
- Modify: `web/package.json` (already added `gen:api` script in Task 1)

- [ ] **Step 1: Run the generator**

From `web/`:
```bash
pnpm gen:api
```

Expected: produces `web/lib/api-types.ts` with a `paths` and `components` type tree derived from `api/openapi.yaml`.

- [ ] **Step 2: Sanity-check the output compiles**

```bash
pnpm typecheck
```

Expected: exit 0.

- [ ] **Step 3: Add a regeneration check to README of `web/`**

Create `web/README.md`:

```markdown
# ccx dashboard

Next.js 15 static-export dashboard for ccx. Embedded by the Go binary at build
time via `//go:embed`.

## Development

```bash
pnpm install
pnpm dev          # http://localhost:3001 — MSW serves mock data
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
```

- [ ] **Step 4: Commit**

```bash
git add web/lib/api-types.ts web/README.md
git commit -m "feat(web): generate typed API surface from openapi.yaml"
```

---

## Task 3: Add a CI guard that fails if api-types.ts is stale

**Files:**
- Create: `web/scripts/check-api-types.sh`

- [ ] **Step 1: Write the check script**

Create `web/scripts/check-api-types.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

# Regenerate types into a temp file and diff against the committed copy.
TMP=$(mktemp)
trap "rm -f $TMP" EXIT

pnpm exec openapi-typescript ../api/openapi.yaml -o "$TMP" >/dev/null

if ! diff -q "$TMP" ./lib/api-types.ts >/dev/null 2>&1; then
  echo "::error::lib/api-types.ts is stale. Run 'pnpm gen:api' and commit." >&2
  diff -u ./lib/api-types.ts "$TMP" || true
  exit 1
fi

echo "api-types.ts is up to date."
```

Make it executable:
```bash
chmod +x web/scripts/check-api-types.sh
```

- [ ] **Step 2: Wire it into a `package.json` script**

Edit `web/package.json` to add to `scripts`:
```json
"check:api-types": "bash scripts/check-api-types.sh"
```

- [ ] **Step 3: Run it to confirm green**

```bash
pnpm check:api-types
```

Expected: prints `api-types.ts is up to date.`

- [ ] **Step 4: Commit**

```bash
git add web/scripts/check-api-types.sh web/package.json
git commit -m "ci(web): guard against stale api-types.ts"
```

---

## Task 4: Build the typed API client wrapper (TDD)

**Files:**
- Create: `web/lib/api.ts`
- Create: `web/lib/api.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/lib/api.test.ts`:

```ts
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { getHealth, getProfiles, getUsage, apiBaseUrl } from './api';

describe('api client', () => {
  const originalFetch = global.fetch;

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it('exposes the API base URL, defaulting to 127.0.0.1:7777', () => {
    expect(apiBaseUrl()).toMatch(/^https?:\/\//);
  });

  it('getHealth parses { ok, version } from /api/health', async () => {
    global.fetch = vi.fn(async () =>
      new Response(JSON.stringify({ ok: true, version: '0.1.0-dev' }), {
        headers: { 'Content-Type': 'application/json' },
      }),
    ) as unknown as typeof fetch;

    const out = await getHealth();
    expect(out.ok).toBe(true);
    expect(out.version).toBe('0.1.0-dev');
  });

  it('getProfiles parses an array of ProfileWithTotals', async () => {
    global.fetch = vi.fn(async () =>
      new Response(
        JSON.stringify([
          {
            name: 'work',
            config_dir: '/x',
            color: '#3B82F6',
            created_at: '2026-05-19T12:00:00Z',
            last_used_at: '2026-05-19T12:00:00Z',
            today: {
              usage: {
                input_tokens: 1,
                output_tokens: 2,
                cache_read_tokens: 3,
                cache_create_tokens: 4,
              },
              estimated_usd: 0.42,
            },
          },
        ]),
        { headers: { 'Content-Type': 'application/json' } },
      ),
    ) as unknown as typeof fetch;

    const out = await getProfiles();
    expect(out).toHaveLength(1);
    expect(out[0]?.name).toBe('work');
    expect(out[0]?.today.estimated_usd).toBe(0.42);
  });

  it('getUsage forwards query params and parses rows', async () => {
    const spy = vi.fn(async (input: RequestInfo | URL) => {
      const url = typeof input === 'string' ? input : input.toString();
      expect(url).toContain('/api/usage');
      expect(url).toContain('profile=work');
      expect(url).toContain('since=7d');
      return new Response(
        JSON.stringify({
          rows: [],
          total: {
            usage: {
              input_tokens: 0,
              output_tokens: 0,
              cache_read_tokens: 0,
              cache_create_tokens: 0,
            },
            estimated_usd: 0,
          },
        }),
        { headers: { 'Content-Type': 'application/json' } },
      );
    });
    global.fetch = spy as unknown as typeof fetch;

    const out = await getUsage({ profile: 'work', since: '7d' });
    expect(out.rows).toEqual([]);
    expect(out.total.estimated_usd).toBe(0);
    expect(spy).toHaveBeenCalledOnce();
  });

  it('throws on non-2xx with a useful message', async () => {
    global.fetch = vi.fn(async () =>
      new Response('boom', { status: 500 }),
    ) as unknown as typeof fetch;

    await expect(getHealth()).rejects.toThrow(/500/);
  });
});
```

- [ ] **Step 2: Verify the test currently fails**

```bash
pnpm test -- lib/api.test.ts
```

Expected: FAIL with "cannot find module ./api" (file doesn't exist yet). Vitest may not be configured yet; if Vitest is missing, finish Task 9 first then return — but we keep test files defined now so the structure is locked in. If you would prefer strict ordering, skip running this until Task 9 and just commit the test file. Either way the test code stays as written.

- [ ] **Step 3: Write the implementation**

Create `web/lib/api.ts`:

```ts
import type { components, paths } from './api-types';

export type Profile = components['schemas']['Profile'];
export type ProfileWithTotals = components['schemas']['ProfileWithTotals'];
export type Usage = components['schemas']['Usage'];
export type UsageRow = components['schemas']['UsageRow'];
export type UsageTotal = components['schemas']['UsageTotal'];

export interface HealthResponse {
  ok: boolean;
  version: string;
}

export interface UsageResponse {
  rows: UsageRow[];
  total: UsageTotal;
}

export interface GetUsageParams {
  profile?: string;
  project?: string;
  /** Duration like "24h", "7d", "30d". Default "24h" on the server. */
  since?: string;
}

const DEFAULT_BASE = 'http://127.0.0.1:7777';

/** API base URL. Reads NEXT_PUBLIC_API_BASE at build time, falls back to localhost. */
export function apiBaseUrl(): string {
  const env =
    typeof process !== 'undefined'
      ? process.env.NEXT_PUBLIC_API_BASE
      : undefined;
  return (env && env.length > 0 ? env : DEFAULT_BASE).replace(/\/$/, '');
}

async function getJSON<T>(path: string): Promise<T> {
  const url = `${apiBaseUrl()}${path}`;
  const res = await fetch(url, {
    headers: { Accept: 'application/json' },
    cache: 'no-store',
  });
  if (!res.ok) {
    const body = await res.text().catch(() => '');
    throw new Error(
      `ccx API ${res.status} on ${path}: ${body.slice(0, 200) || res.statusText}`,
    );
  }
  return (await res.json()) as T;
}

export async function getHealth(): Promise<HealthResponse> {
  return getJSON<HealthResponse>('/api/health');
}

export async function getProfiles(): Promise<ProfileWithTotals[]> {
  return getJSON<ProfileWithTotals[]>('/api/profiles');
}

export async function getUsage(params: GetUsageParams = {}): Promise<UsageResponse> {
  const qs = new URLSearchParams();
  if (params.profile) qs.set('profile', params.profile);
  if (params.project) qs.set('project', params.project);
  if (params.since) qs.set('since', params.since);
  const suffix = qs.toString() ? `?${qs.toString()}` : '';
  return getJSON<UsageResponse>(`/api/usage${suffix}`);
}

/**
 * streamUsage opens an SSE connection to /api/usage/live and invokes onRow
 * for each emitted UsageRow array. Returns a teardown function.
 *
 * In Phase 1 this is mocked by MSW (which can simulate SSE via ReadableStream).
 * In production it talks to the Go server.
 */
export function streamUsage(
  onRows: (rows: UsageRow[]) => void,
  onError?: (err: Error) => void,
): () => void {
  const url = `${apiBaseUrl()}/api/usage/live`;
  const es = new EventSource(url);
  es.addEventListener('usage', (ev) => {
    try {
      const parsed = JSON.parse((ev as MessageEvent).data) as UsageRow[];
      onRows(parsed);
    } catch (e) {
      onError?.(e as Error);
    }
  });
  es.onerror = () => {
    onError?.(new Error('SSE connection error'));
  };
  return () => es.close();
}

// Compile-time check: ensure our manual response shapes are still aligned with
// the generated OpenAPI types. If a contract change breaks these, TypeScript
// errors here force a regeneration.
type _HealthCheck = paths['/api/health']['get']['responses']['200']['content']['application/json'];
type _ProfilesCheck = paths['/api/profiles']['get']['responses']['200']['content']['application/json'];
type _UsageCheck = paths['/api/usage']['get']['responses']['200']['content']['application/json'];
// eslint-disable-next-line @typescript-eslint/no-unused-vars
type _Assert = [_HealthCheck, _ProfilesCheck, _UsageCheck];
```

- [ ] **Step 4: Run the test**

After Vitest is set up (Task 9), `pnpm test -- lib/api.test.ts` should pass. If running in this task, expect PASS.

- [ ] **Step 5: Commit**

```bash
git add web/lib/api.ts web/lib/api.test.ts
git commit -m "feat(web): add typed API client wrapper"
```

---

## Task 5: Add deterministic profile color helper (TDD)

**Files:**
- Create: `web/lib/profile-color.ts`
- Create: `web/lib/profile-color.test.ts`

- [ ] **Step 1: Failing test**

Create `web/lib/profile-color.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { profileAccent } from './profile-color';

describe('profileAccent', () => {
  it('returns the profile color when set', () => {
    expect(profileAccent({ name: 'work', color: '#3B82F6' })).toBe('#3B82F6');
  });

  it('returns a stable hex for the same name when color is missing', () => {
    const a = profileAccent({ name: 'work' });
    const b = profileAccent({ name: 'work' });
    expect(a).toBe(b);
    expect(a).toMatch(/^#[0-9A-Fa-f]{6}$/);
  });

  it('returns different colors for different names', () => {
    expect(profileAccent({ name: 'work' })).not.toBe(profileAccent({ name: 'personal' }));
  });
});
```

- [ ] **Step 2: Implementation**

Create `web/lib/profile-color.ts`:

```ts
// Curated 8-color palette tuned for dark and light modes (Tailwind 500-shades).
const PALETTE = [
  '#3B82F6', // blue
  '#10B981', // emerald
  '#F59E0B', // amber
  '#EF4444', // red
  '#8B5CF6', // violet
  '#EC4899', // pink
  '#06B6D4', // cyan
  '#84CC16', // lime
] as const;

function hash(str: string): number {
  let h = 2166136261;
  for (let i = 0; i < str.length; i++) {
    h ^= str.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  return h >>> 0;
}

export interface ColorableProfile {
  name: string;
  color?: string;
}

/** Returns a stable accent color for the profile. Prefers profile.color if set. */
export function profileAccent(p: ColorableProfile): string {
  if (p.color && /^#[0-9A-Fa-f]{6}$/.test(p.color)) return p.color;
  return PALETTE[hash(p.name) % PALETTE.length]!;
}
```

- [ ] **Step 3: Run the test**

```bash
pnpm test -- lib/profile-color.test.ts
```

Expected: PASS (after Vitest is set up in Task 9; commit now and verify after).

- [ ] **Step 4: Commit**

```bash
git add web/lib/profile-color.ts web/lib/profile-color.test.ts
git commit -m "feat(web): deterministic profile accent color"
```

---

## Task 6: Wire Tailwind, fonts, and base CSS

**Files:**
- Modify: `web/app/globals.css`
- Modify: `web/app/layout.tsx`
- Modify: `web/tailwind.config.ts`

- [ ] **Step 1: Replace `web/app/globals.css`**

```css
@tailwind base;
@tailwind components;
@tailwind utilities;

:root {
  --background: #ffffff;
  --foreground: #0a0a0a;
  --muted: #6b7280;
  --card: #ffffff;
  --card-border: #e5e7eb;
  --grid: #f3f4f6;
}

.dark {
  --background: #0a0a0a;
  --foreground: #f5f5f5;
  --muted: #a1a1aa;
  --card: #111113;
  --card-border: #27272a;
  --grid: #1f1f23;
}

html,
body {
  background: var(--background);
  color: var(--foreground);
}

body {
  font-family: var(--font-inter), ui-sans-serif, system-ui, sans-serif;
  font-feature-settings: 'cv02', 'cv11';
  -webkit-font-smoothing: antialiased;
}

.font-mono,
.tabular {
  font-family: var(--font-mono), ui-monospace, SFMono-Regular, monospace;
  font-variant-numeric: tabular-nums;
}
```

- [ ] **Step 2: Update `web/tailwind.config.ts`**

```ts
import type { Config } from 'tailwindcss';

const config: Config = {
  darkMode: 'class',
  content: [
    './app/**/*.{ts,tsx}',
    './components/**/*.{ts,tsx}',
    './lib/**/*.{ts,tsx}',
  ],
  theme: {
    extend: {
      colors: {
        background: 'var(--background)',
        foreground: 'var(--foreground)',
        muted: 'var(--muted)',
        card: 'var(--card)',
        'card-border': 'var(--card-border)',
        grid: 'var(--grid)',
      },
      fontFamily: {
        sans: ['var(--font-inter)', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        mono: ['var(--font-mono)', 'ui-monospace', 'SFMono-Regular', 'monospace'],
      },
    },
  },
  plugins: [],
};

export default config;
```

- [ ] **Step 3: Replace `web/app/layout.tsx`**

```tsx
import type { Metadata } from 'next';
import { Inter, JetBrains_Mono } from 'next/font/google';
import './globals.css';
import { ThemeProvider } from '@/components/theme-provider';
import { MswBoot } from '@/components/msw-boot';

const inter = Inter({
  subsets: ['latin'],
  variable: '--font-inter',
  display: 'swap',
});

const jetbrains = JetBrains_Mono({
  subsets: ['latin'],
  variable: '--font-mono',
  display: 'swap',
});

export const metadata: Metadata = {
  title: 'ccx dashboard',
  description: 'Multi-account Claude Code workspace usage dashboard',
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${inter.variable} ${jetbrains.variable}`} suppressHydrationWarning>
      <body className="min-h-screen bg-background text-foreground antialiased">
        <ThemeProvider>
          <MswBoot />
          {children}
        </ThemeProvider>
      </body>
    </html>
  );
}
```

(We'll create `theme-provider` and `msw-boot` in upcoming tasks. The build will fail until those exist — that's fine, those tasks come next.)

- [ ] **Step 4: Commit**

```bash
git add web/app/globals.css web/tailwind.config.ts web/app/layout.tsx
git commit -m "feat(web): tailwind config, fonts, css variables"
```

---

## Task 7: Add the dark-mode ThemeProvider and ToggleThemeButton

**Files:**
- Create: `web/components/theme-provider.tsx`
- Create: `web/components/toggle-theme.tsx`

- [ ] **Step 1: Theme provider**

Create `web/components/theme-provider.tsx`:

```tsx
'use client';

import { ThemeProvider as NextThemeProvider } from 'next-themes';
import type { ReactNode } from 'react';

export function ThemeProvider({ children }: { children: ReactNode }) {
  return (
    <NextThemeProvider attribute="class" defaultTheme="dark" enableSystem={false}>
      {children}
    </NextThemeProvider>
  );
}
```

- [ ] **Step 2: Toggle button**

Create `web/components/toggle-theme.tsx`:

```tsx
'use client';

import { useEffect, useState } from 'react';
import { useTheme } from 'next-themes';
import { Moon, Sun } from 'lucide-react';

export function ToggleTheme() {
  const { theme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);
  if (!mounted) return <div className="h-8 w-8" aria-hidden />;

  const isDark = theme === 'dark';
  return (
    <button
      type="button"
      onClick={() => setTheme(isDark ? 'light' : 'dark')}
      aria-label={isDark ? 'Switch to light mode' : 'Switch to dark mode'}
      className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-card-border bg-card text-foreground hover:bg-grid"
    >
      {isDark ? <Sun size={16} /> : <Moon size={16} />}
    </button>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add web/components/theme-provider.tsx web/components/toggle-theme.tsx
git commit -m "feat(web): dark mode provider and toggle"
```

---

## Task 8: Set up MSW handlers + dev-only boot component

**Files:**
- Create: `web/mocks/handlers.ts`
- Create: `web/mocks/browser.ts`
- Create: `web/mocks/fixtures.ts`
- Create: `web/components/msw-boot.tsx`
- Create: `web/public/mockServiceWorker.js` (auto-generated)

- [ ] **Step 1: Install MSW worker script**

```bash
cd web && pnpm exec msw init public/ --save
```

This creates `web/public/mockServiceWorker.js`. The `--save` flag also records the worker location in `package.json`.

- [ ] **Step 2: Build fixtures**

Create `web/mocks/fixtures.ts`:

```ts
import type { ProfileWithTotals, UsageRow, UsageTotal } from '@/lib/api';

const now = Date.UTC(2026, 4, 19, 12, 0, 0);
const day = 24 * 60 * 60 * 1000;

function isoDay(offsetDays: number): string {
  return new Date(now - offsetDays * day).toISOString();
}

function usage(input: number, output: number, cacheR: number, cacheC: number) {
  return {
    input_tokens: input,
    output_tokens: output,
    cache_read_tokens: cacheR,
    cache_create_tokens: cacheC,
  };
}

function total(input: number, output: number, cacheR: number, cacheC: number, usd: number): UsageTotal {
  return { usage: usage(input, output, cacheR, cacheC), estimated_usd: usd };
}

export const FIXTURE_PROFILES: ProfileWithTotals[] = [
  {
    name: 'work',
    config_dir: '/Users/arafa/.claude-profiles/work',
    label: 'Work account',
    color: '#3B82F6',
    created_at: '2026-04-01T10:00:00Z',
    last_used_at: isoDay(0),
    today: total(1_200_000, 240_000, 4_100_000, 60_000, 18.42),
  },
  {
    name: 'personal',
    config_dir: '/Users/arafa/.claude-profiles/personal',
    label: 'Personal',
    color: '#10B981',
    created_at: '2026-04-05T10:00:00Z',
    last_used_at: isoDay(0),
    today: total(220_000, 84_000, 980_000, 12_000, 4.18),
  },
  {
    name: 'side',
    config_dir: '/Users/arafa/.claude-profiles/side',
    label: 'Side project',
    color: '#F59E0B',
    created_at: '2026-05-10T10:00:00Z',
    last_used_at: isoDay(1),
    today: total(80_000, 30_000, 200_000, 5_000, 1.12),
  },
];

const PROJECTS = ['ccx', 'acme-api', 'hobby-site', 'experiments', 'devops'];

export function generateUsageRows(profileFilter?: string): UsageRow[] {
  const rows: UsageRow[] = [];
  for (const profile of FIXTURE_PROFILES) {
    if (profileFilter && profile.name !== profileFilter) continue;
    for (let d = 6; d >= 0; d--) {
      // Multiple projects per day for richer table data
      for (let p = 0; p < 2; p++) {
        const project = PROJECTS[(d + p + profile.name.length) % PROJECTS.length]!;
        const scale = profile.name === 'work' ? 1.0 : profile.name === 'personal' ? 0.35 : 0.15;
        const input = Math.round((80_000 + Math.sin(d + p) * 30_000) * scale);
        const output = Math.round((20_000 + Math.cos(d + p) * 8_000) * scale);
        const cacheR = Math.round(input * 3.5);
        const cacheC = Math.round(input * 0.05);
        const usd =
          (input / 1_000_000) * 3 +
          (output / 1_000_000) * 15 +
          (cacheR / 1_000_000) * 0.3 +
          (cacheC / 1_000_000) * 3.75;
        rows.push({
          profile: profile.name,
          project,
          model: d % 2 === 0 ? 'claude-opus-4-7' : 'claude-sonnet-4-6',
          day: isoDay(d),
          usage: usage(input, output, cacheR, cacheC),
          session_count: 1 + ((d + p) % 4),
          estimated_usd: Number(usd.toFixed(2)),
        });
      }
    }
  }
  return rows;
}

export function aggregateTotal(rows: UsageRow[]): UsageTotal {
  const usd = rows.reduce((s, r) => s + r.estimated_usd, 0);
  const u = rows.reduce(
    (s, r) => ({
      input_tokens: s.input_tokens + r.usage.input_tokens,
      output_tokens: s.output_tokens + r.usage.output_tokens,
      cache_read_tokens: s.cache_read_tokens + r.usage.cache_read_tokens,
      cache_create_tokens: s.cache_create_tokens + r.usage.cache_create_tokens,
    }),
    { input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_create_tokens: 0 },
  );
  return { usage: u, estimated_usd: Number(usd.toFixed(2)) };
}
```

- [ ] **Step 3: Handlers**

Create `web/mocks/handlers.ts`:

```ts
import { http, HttpResponse } from 'msw';
import { FIXTURE_PROFILES, generateUsageRows, aggregateTotal } from './fixtures';

const base = (process.env.NEXT_PUBLIC_API_BASE ?? 'http://127.0.0.1:7777').replace(/\/$/, '');

export const handlers = [
  http.get(`${base}/api/health`, () =>
    HttpResponse.json({ ok: true, version: '0.1.0-dev-msw' }),
  ),

  http.get(`${base}/api/profiles`, () => HttpResponse.json(FIXTURE_PROFILES)),

  http.get(`${base}/api/usage`, ({ request }) => {
    const url = new URL(request.url);
    const profile = url.searchParams.get('profile') ?? undefined;
    const rows = generateUsageRows(profile);
    const total = aggregateTotal(rows);
    return HttpResponse.json({ rows, total });
  }),

  http.get(`${base}/api/usage/live`, () => {
    // Simulate a one-shot SSE event. The browser EventSource keeps the
    // connection open; for the static mock we just emit once and let the
    // dev session move on.
    const stream = new ReadableStream({
      start(controller) {
        const rows = generateUsageRows();
        const payload = `event: usage\ndata: ${JSON.stringify(rows)}\n\n`;
        controller.enqueue(new TextEncoder().encode(payload));
        controller.close();
      },
    });
    return new HttpResponse(stream, {
      headers: {
        'Content-Type': 'text/event-stream',
        'Cache-Control': 'no-cache',
        Connection: 'keep-alive',
      },
    });
  }),
];
```

- [ ] **Step 4: Browser entry**

Create `web/mocks/browser.ts`:

```ts
import { setupWorker } from 'msw/browser';
import { handlers } from './handlers';

export const worker = setupWorker(...handlers);
```

- [ ] **Step 5: Dev-only boot component**

Create `web/components/msw-boot.tsx`:

```tsx
'use client';

import { useEffect, useState } from 'react';

const ENABLED = process.env.NODE_ENV === 'development';

export function MswBoot() {
  const [ready, setReady] = useState(!ENABLED);

  useEffect(() => {
    if (!ENABLED) return;
    let cancelled = false;
    (async () => {
      const { worker } = await import('@/mocks/browser');
      await worker.start({
        onUnhandledRequest: 'bypass',
        serviceWorker: { url: '/mockServiceWorker.js' },
      });
      if (!cancelled) setReady(true);
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // The component renders nothing — it just guarantees MSW boots before
  // any dashboard data fetch fires. Children downstream may use a Suspense
  // boundary or render guard, but since fetches happen on user interaction
  // (and on a delay), this minimal "fire-and-forget" boot is sufficient.
  if (!ready) return null;
  return null;
}
```

- [ ] **Step 6: Add MSW init to `.gitignore` exemption if needed**

`public/mockServiceWorker.js` is a generated file but is **committed** because it is needed at build time. Confirm it is not in `.gitignore`.

- [ ] **Step 7: Commit**

```bash
git add web/mocks/ web/components/msw-boot.tsx web/public/mockServiceWorker.js web/package.json
git commit -m "feat(web): mock service worker handlers and dev boot"
```

---

## Task 9: Configure Vitest + Testing Library

**Files:**
- Create: `web/vitest.config.ts`
- Create: `web/vitest.setup.ts`

- [ ] **Step 1: vitest config**

Create `web/vitest.config.ts`:

```ts
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import path from 'node:path';

export default defineConfig({
  plugins: [react()],
  test: {
    environment: 'jsdom',
    globals: false,
    setupFiles: ['./vitest.setup.ts'],
    include: ['**/*.test.{ts,tsx}'],
    exclude: ['node_modules', '.next', 'out', 'e2e/**'],
    css: false,
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, '.'),
    },
  },
});
```

- [ ] **Step 2: Setup file**

Create `web/vitest.setup.ts`:

```ts
import '@testing-library/jest-dom/vitest';
import { afterEach } from 'vitest';
import { cleanup } from '@testing-library/react';

// jsdom does not implement matchMedia; next-themes uses it.
if (typeof window !== 'undefined' && !window.matchMedia) {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: (query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: () => {},
      removeListener: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
      dispatchEvent: () => false,
    }),
  });
}

// jsdom lacks ResizeObserver; Recharts uses it.
if (typeof window !== 'undefined' && !('ResizeObserver' in window)) {
  (window as unknown as { ResizeObserver: typeof ResizeObserver }).ResizeObserver =
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof ResizeObserver;
}

afterEach(() => {
  cleanup();
});
```

- [ ] **Step 3: Run the suite (api + profile-color tests should now pass)**

```bash
cd web && pnpm test
```

Expected: PASS for `lib/api.test.ts` and `lib/profile-color.test.ts`. No other tests yet.

- [ ] **Step 4: Commit**

```bash
git add web/vitest.config.ts web/vitest.setup.ts
git commit -m "test(web): vitest + testing-library setup"
```

---

## Task 10: Build the Header component (TDD)

**Files:**
- Create: `web/components/header.tsx`
- Create: `web/components/header.test.tsx`

The header bar contains: ccx wordmark, profile picker dropdown (filter-only — does not switch the active profile), live-status dot, and the theme toggle.

- [ ] **Step 1: Failing test**

Create `web/components/header.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Header } from './header';
import { ThemeProvider } from './theme-provider';

const profiles = [
  { name: 'work', color: '#3B82F6' },
  { name: 'personal', color: '#10B981' },
  { name: 'side', color: '#F59E0B' },
];

function renderHeader(props: Partial<React.ComponentProps<typeof Header>> = {}) {
  const onSelect = vi.fn();
  render(
    <ThemeProvider>
      <Header
        profiles={profiles}
        selected={null}
        onSelect={onSelect}
        live="connected"
        {...props}
      />
    </ThemeProvider>,
  );
  return { onSelect };
}

describe('<Header>', () => {
  it('renders the ccx wordmark', () => {
    renderHeader();
    expect(screen.getByText(/ccx/i)).toBeInTheDocument();
  });

  it('lists all profiles in the picker', async () => {
    renderHeader();
    await userEvent.click(screen.getByRole('button', { name: /filter/i }));
    expect(screen.getByRole('menuitem', { name: /all profiles/i })).toBeInTheDocument();
    for (const p of profiles) {
      expect(screen.getByRole('menuitem', { name: new RegExp(p.name, 'i') })).toBeInTheDocument();
    }
  });

  it('calls onSelect when a profile is chosen', async () => {
    const { onSelect } = renderHeader();
    await userEvent.click(screen.getByRole('button', { name: /filter/i }));
    await userEvent.click(screen.getByRole('menuitem', { name: /work/i }));
    expect(onSelect).toHaveBeenCalledWith('work');
  });

  it('shows a green live-status dot when connected', () => {
    renderHeader({ live: 'connected' });
    expect(screen.getByLabelText(/live updates connected/i)).toBeInTheDocument();
  });

  it('shows a gray live-status dot when disconnected', () => {
    renderHeader({ live: 'disconnected' });
    expect(screen.getByLabelText(/live updates disconnected/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Implementation**

Create `web/components/header.tsx`:

```tsx
'use client';

import { useState } from 'react';
import { ChevronDown, Circle } from 'lucide-react';
import { ToggleTheme } from './toggle-theme';
import { profileAccent } from '@/lib/profile-color';

export type LiveStatus = 'connected' | 'disconnected' | 'connecting';

export interface HeaderProfile {
  name: string;
  color?: string;
}

export interface HeaderProps {
  profiles: HeaderProfile[];
  selected: string | null;
  onSelect: (name: string | null) => void;
  live: LiveStatus;
}

export function Header({ profiles, selected, onSelect, live }: HeaderProps) {
  const [open, setOpen] = useState(false);
  const selectedProfile = profiles.find((p) => p.name === selected) ?? null;

  return (
    <header className="sticky top-0 z-20 flex items-center justify-between border-b border-card-border bg-background/80 px-6 py-3 backdrop-blur">
      <div className="flex items-center gap-3">
        <span className="font-mono text-lg font-semibold tracking-tight">ccx</span>
        <span className="text-xs text-muted">dashboard</span>
      </div>

      <div className="flex items-center gap-3">
        <div className="relative">
          <button
            type="button"
            onClick={() => setOpen((v) => !v)}
            aria-label="Filter by profile"
            aria-haspopup="menu"
            aria-expanded={open}
            className="inline-flex h-8 items-center gap-2 rounded-md border border-card-border bg-card px-3 text-sm hover:bg-grid"
          >
            {selectedProfile && (
              <span
                aria-hidden
                className="h-2 w-2 rounded-full"
                style={{ background: profileAccent(selectedProfile) }}
              />
            )}
            <span>{selectedProfile ? selectedProfile.name : 'All profiles'}</span>
            <ChevronDown size={14} />
          </button>
          {open && (
            <div
              role="menu"
              className="absolute right-0 mt-2 w-48 rounded-md border border-card-border bg-card py-1 shadow-lg"
            >
              <button
                role="menuitem"
                type="button"
                onClick={() => {
                  onSelect(null);
                  setOpen(false);
                }}
                className="flex w-full items-center gap-2 px-3 py-1.5 text-sm hover:bg-grid"
              >
                <span className="h-2 w-2 rounded-full bg-muted" aria-hidden />
                All profiles
              </button>
              {profiles.map((p) => (
                <button
                  key={p.name}
                  role="menuitem"
                  type="button"
                  onClick={() => {
                    onSelect(p.name);
                    setOpen(false);
                  }}
                  className="flex w-full items-center gap-2 px-3 py-1.5 text-sm hover:bg-grid"
                >
                  <span
                    aria-hidden
                    className="h-2 w-2 rounded-full"
                    style={{ background: profileAccent(p) }}
                  />
                  {p.name}
                </button>
              ))}
            </div>
          )}
        </div>

        <span
          aria-label={
            live === 'connected'
              ? 'Live updates connected'
              : live === 'connecting'
                ? 'Live updates connecting'
                : 'Live updates disconnected'
          }
          className="inline-flex items-center gap-1.5 text-xs text-muted"
        >
          <Circle
            size={8}
            fill={live === 'connected' ? '#22c55e' : live === 'connecting' ? '#f59e0b' : '#71717a'}
            stroke="none"
          />
          {live === 'connected' ? 'live' : live === 'connecting' ? '…' : 'offline'}
        </span>

        <ToggleTheme />
      </div>
    </header>
  );
}
```

- [ ] **Step 3: Run test**

```bash
pnpm test -- components/header.test.tsx
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/components/header.tsx web/components/header.test.tsx
git commit -m "feat(web): header with profile picker and live-status dot"
```

---

## Task 11: Build ProfileCard + ProfileCards row (TDD)

**Files:**
- Create: `web/components/profile-card.tsx`
- Create: `web/components/profile-cards.tsx`
- Create: `web/components/profile-cards.test.tsx`

A row of cards, one per profile. Each shows: profile name, today's spend (USD), today's total tokens (formatted compact), and a 7-day sparkline.

- [ ] **Step 1: Failing test**

Create `web/components/profile-cards.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ProfileCards } from './profile-cards';
import type { ProfileWithTotals, UsageRow } from '@/lib/api';

const profiles: ProfileWithTotals[] = [
  {
    name: 'work',
    config_dir: '/x',
    color: '#3B82F6',
    created_at: '2026-05-01T00:00:00Z',
    last_used_at: '2026-05-19T00:00:00Z',
    today: {
      usage: {
        input_tokens: 1_000_000,
        output_tokens: 200_000,
        cache_read_tokens: 4_000_000,
        cache_create_tokens: 50_000,
      },
      estimated_usd: 12.34,
    },
  },
];

const rows: UsageRow[] = Array.from({ length: 7 }, (_, i) => ({
  profile: 'work',
  project: 'ccx',
  model: 'claude-opus-4-7',
  day: new Date(Date.UTC(2026, 4, 13 + i)).toISOString(),
  usage: {
    input_tokens: 1000 * (i + 1),
    output_tokens: 200 * (i + 1),
    cache_read_tokens: 0,
    cache_create_tokens: 0,
  },
  session_count: 1,
  estimated_usd: 0.5 * (i + 1),
}));

describe('<ProfileCards>', () => {
  it('renders one card per profile', () => {
    render(<ProfileCards profiles={profiles} usageRows={rows} />);
    expect(screen.getByText('work')).toBeInTheDocument();
    expect(screen.getByText('$12.34')).toBeInTheDocument();
  });

  it('renders nothing gracefully when profile list is empty', () => {
    const { container } = render(<ProfileCards profiles={[]} usageRows={[]} />);
    expect(container.querySelectorAll('[data-testid="profile-card"]').length).toBe(0);
  });

  it('formats total tokens in compact notation', () => {
    render(<ProfileCards profiles={profiles} usageRows={rows} />);
    // 1M + 200k + 4M + 50k = 5.25M
    expect(screen.getByText(/5\.2[0-9]M/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: ProfileCard component**

Create `web/components/profile-card.tsx`:

```tsx
'use client';

import { LineChart, Line, ResponsiveContainer } from 'recharts';
import type { ProfileWithTotals, UsageRow } from '@/lib/api';
import { profileAccent } from '@/lib/profile-color';

export interface ProfileCardProps {
  profile: ProfileWithTotals;
  sparkline: { day: string; tokens: number }[];
}

const compact = new Intl.NumberFormat('en-US', { notation: 'compact', maximumFractionDigits: 2 });
const usd = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' });

export function ProfileCard({ profile, sparkline }: ProfileCardProps) {
  const accent = profileAccent(profile);
  const totalTokens =
    profile.today.usage.input_tokens +
    profile.today.usage.output_tokens +
    profile.today.usage.cache_read_tokens +
    profile.today.usage.cache_create_tokens;

  return (
    <div
      data-testid="profile-card"
      className="flex flex-col gap-3 rounded-xl border border-card-border bg-card p-4 transition hover:shadow-md"
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span aria-hidden className="h-2.5 w-2.5 rounded-full" style={{ background: accent }} />
          <span className="text-sm font-medium">{profile.name}</span>
        </div>
        {profile.label && (
          <span className="text-xs text-muted">{profile.label}</span>
        )}
      </div>

      <div className="flex items-baseline justify-between">
        <span className="font-mono text-2xl tabular tracking-tight">
          {usd.format(profile.today.estimated_usd)}
        </span>
        <span className="font-mono text-xs tabular text-muted">{compact.format(totalTokens)} tok</span>
      </div>

      <div className="h-10 -mx-1">
        <ResponsiveContainer width="100%" height="100%">
          <LineChart data={sparkline} margin={{ top: 4, right: 4, bottom: 4, left: 4 }}>
            <Line
              type="monotone"
              dataKey="tokens"
              stroke={accent}
              strokeWidth={1.5}
              dot={false}
              isAnimationActive={false}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: ProfileCards row**

Create `web/components/profile-cards.tsx`:

```tsx
'use client';

import { useMemo } from 'react';
import { ProfileCard } from './profile-card';
import type { ProfileWithTotals, UsageRow } from '@/lib/api';

export interface ProfileCardsProps {
  profiles: ProfileWithTotals[];
  usageRows: UsageRow[];
}

export function ProfileCards({ profiles, usageRows }: ProfileCardsProps) {
  const sparklinesByProfile = useMemo(() => {
    const byProfile = new Map<string, Map<string, number>>();
    for (const row of usageRows) {
      const day = row.day.slice(0, 10);
      const inner = byProfile.get(row.profile) ?? new Map<string, number>();
      const total =
        row.usage.input_tokens +
        row.usage.output_tokens +
        row.usage.cache_read_tokens +
        row.usage.cache_create_tokens;
      inner.set(day, (inner.get(day) ?? 0) + total);
      byProfile.set(row.profile, inner);
    }
    const out = new Map<string, { day: string; tokens: number }[]>();
    for (const [profile, days] of byProfile) {
      const series = Array.from(days.entries())
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([day, tokens]) => ({ day, tokens }));
      out.set(profile, series);
    }
    return out;
  }, [usageRows]);

  if (profiles.length === 0) return null;

  return (
    <section
      aria-label="Profiles"
      className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
    >
      {profiles.map((p) => (
        <ProfileCard
          key={p.name}
          profile={p}
          sparkline={sparklinesByProfile.get(p.name) ?? []}
        />
      ))}
    </section>
  );
}
```

- [ ] **Step 4: Run tests**

```bash
pnpm test -- components/profile-cards.test.tsx
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/components/profile-card.tsx web/components/profile-cards.tsx web/components/profile-cards.test.tsx
git commit -m "feat(web): profile cards row with sparklines"
```

---

## Task 12: Build the TimeSeriesChart (TDD)

**Files:**
- Create: `web/components/time-series-chart.tsx`
- Create: `web/components/time-series-chart.test.tsx`

Stacked area chart of daily tokens per profile. Recharts `<AreaChart>` with one stacked `<Area>` per profile. Uses the profile's accent color.

- [ ] **Step 1: Failing test**

Create `web/components/time-series-chart.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import { TimeSeriesChart } from './time-series-chart';
import type { UsageRow } from '@/lib/api';

describe('<TimeSeriesChart>', () => {
  it('renders without crashing on empty data', () => {
    const { container } = render(
      <TimeSeriesChart usageRows={[]} profiles={[{ name: 'work', color: '#3B82F6' }]} />,
    );
    expect(container.querySelector('[data-testid="time-series-chart"]')).toBeInTheDocument();
  });

  it('renders an SVG area element when data is provided', () => {
    const rows: UsageRow[] = [
      {
        profile: 'work',
        day: '2026-05-19T00:00:00Z',
        usage: {
          input_tokens: 1000,
          output_tokens: 500,
          cache_read_tokens: 0,
          cache_create_tokens: 0,
        },
        session_count: 1,
        estimated_usd: 0.5,
      },
      {
        profile: 'work',
        day: '2026-05-18T00:00:00Z',
        usage: {
          input_tokens: 1500,
          output_tokens: 700,
          cache_read_tokens: 0,
          cache_create_tokens: 0,
        },
        session_count: 1,
        estimated_usd: 0.8,
      },
    ];
    const { container } = render(
      <TimeSeriesChart usageRows={rows} profiles={[{ name: 'work', color: '#3B82F6' }]} />,
    );
    // recharts renders an svg
    expect(container.querySelector('svg')).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Implementation**

Create `web/components/time-series-chart.tsx`:

```tsx
'use client';

import { useMemo } from 'react';
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';
import type { UsageRow } from '@/lib/api';
import { profileAccent } from '@/lib/profile-color';

export interface TimeSeriesProfileMeta {
  name: string;
  color?: string;
}

export interface TimeSeriesChartProps {
  usageRows: UsageRow[];
  profiles: TimeSeriesProfileMeta[];
}

interface DayPoint {
  day: string;
  [profileName: string]: number | string;
}

const compact = new Intl.NumberFormat('en-US', { notation: 'compact', maximumFractionDigits: 1 });

export function TimeSeriesChart({ usageRows, profiles }: TimeSeriesChartProps) {
  const data = useMemo<DayPoint[]>(() => {
    if (usageRows.length === 0) return [];
    const byDay = new Map<string, DayPoint>();
    for (const row of usageRows) {
      const day = row.day.slice(0, 10);
      const point = byDay.get(day) ?? ({ day } as DayPoint);
      const total =
        row.usage.input_tokens +
        row.usage.output_tokens +
        row.usage.cache_read_tokens +
        row.usage.cache_create_tokens;
      const prev = (point[row.profile] as number | undefined) ?? 0;
      point[row.profile] = prev + total;
      byDay.set(day, point);
    }
    return Array.from(byDay.values()).sort((a, b) => a.day.localeCompare(b.day));
  }, [usageRows]);

  return (
    <section
      data-testid="time-series-chart"
      aria-label="Daily tokens by profile"
      className="rounded-xl border border-card-border bg-card p-4"
    >
      <h2 className="mb-3 text-sm font-medium">Daily tokens</h2>
      <div className="h-64">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data} margin={{ top: 10, right: 12, left: 0, bottom: 0 }}>
            <CartesianGrid stroke="var(--grid)" vertical={false} />
            <XAxis dataKey="day" stroke="var(--muted)" fontSize={11} tickMargin={6} />
            <YAxis
              stroke="var(--muted)"
              fontSize={11}
              tickFormatter={(v: number) => compact.format(v)}
              width={48}
            />
            <Tooltip
              contentStyle={{
                background: 'var(--card)',
                border: '1px solid var(--card-border)',
                borderRadius: 8,
                fontSize: 12,
              }}
              formatter={(v: number) => compact.format(v)}
            />
            {profiles.map((p) => (
              <Area
                key={p.name}
                type="monotone"
                dataKey={p.name}
                stackId="usage"
                stroke={profileAccent(p)}
                fill={profileAccent(p)}
                fillOpacity={0.25}
                isAnimationActive={false}
              />
            ))}
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </section>
  );
}
```

- [ ] **Step 3: Run test**

```bash
pnpm test -- components/time-series-chart.test.tsx
```

Expected: PASS (ResizeObserver shim from `vitest.setup.ts` is required).

- [ ] **Step 4: Commit**

```bash
git add web/components/time-series-chart.tsx web/components/time-series-chart.test.tsx
git commit -m "feat(web): stacked area chart of daily tokens"
```

---

## Task 13: Build the TopProjects table (TDD)

**Files:**
- Create: `web/components/top-projects.tsx`
- Create: `web/components/top-projects.test.tsx`

Sortable by tokens / cost / sessions. Aggregates UsageRows by `project` (ignoring day) and renders top N rows in a table.

- [ ] **Step 1: Failing test**

Create `web/components/top-projects.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { TopProjects } from './top-projects';
import type { UsageRow } from '@/lib/api';

function row(project: string, tokens: number, usd: number, sessions: number): UsageRow {
  return {
    profile: 'work',
    project,
    day: '2026-05-19T00:00:00Z',
    usage: {
      input_tokens: tokens,
      output_tokens: 0,
      cache_read_tokens: 0,
      cache_create_tokens: 0,
    },
    session_count: sessions,
    estimated_usd: usd,
  };
}

const rows: UsageRow[] = [
  row('acme-api', 500_000, 8.5, 5),
  row('ccx', 1_200_000, 18.2, 12),
  row('hobby-site', 100_000, 1.1, 2),
];

describe('<TopProjects>', () => {
  it('renders rows sorted by tokens desc by default', () => {
    render(<TopProjects usageRows={rows} />);
    const cells = screen.getAllByRole('cell');
    expect(cells[0]).toHaveTextContent('ccx');
  });

  it('sorts by cost when cost header clicked', async () => {
    render(<TopProjects usageRows={rows} />);
    await userEvent.click(screen.getByRole('button', { name: /cost/i }));
    const cells = screen.getAllByRole('cell');
    expect(cells[0]).toHaveTextContent('ccx'); // ccx has highest cost too
  });

  it('sorts by sessions when sessions header clicked', async () => {
    render(<TopProjects usageRows={rows} />);
    await userEvent.click(screen.getByRole('button', { name: /sessions/i }));
    const cells = screen.getAllByRole('cell');
    expect(cells[0]).toHaveTextContent('ccx'); // highest sessions
  });

  it('renders empty-state when no rows', () => {
    render(<TopProjects usageRows={[]} />);
    expect(screen.getByText(/no projects yet/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Implementation**

Create `web/components/top-projects.tsx`:

```tsx
'use client';

import { useMemo, useState } from 'react';
import { ArrowDown } from 'lucide-react';
import type { UsageRow } from '@/lib/api';

export interface TopProjectsProps {
  usageRows: UsageRow[];
  limit?: number;
}

type SortKey = 'tokens' | 'cost' | 'sessions';

interface Aggregated {
  project: string;
  tokens: number;
  cost: number;
  sessions: number;
}

const compact = new Intl.NumberFormat('en-US', { notation: 'compact', maximumFractionDigits: 2 });
const usd = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' });

export function TopProjects({ usageRows, limit = 10 }: TopProjectsProps) {
  const [sort, setSort] = useState<SortKey>('tokens');

  const aggregated = useMemo<Aggregated[]>(() => {
    const map = new Map<string, Aggregated>();
    for (const r of usageRows) {
      if (!r.project) continue;
      const total =
        r.usage.input_tokens +
        r.usage.output_tokens +
        r.usage.cache_read_tokens +
        r.usage.cache_create_tokens;
      const cur = map.get(r.project) ?? { project: r.project, tokens: 0, cost: 0, sessions: 0 };
      cur.tokens += total;
      cur.cost += r.estimated_usd;
      cur.sessions += r.session_count;
      map.set(r.project, cur);
    }
    const arr = Array.from(map.values());
    arr.sort((a, b) => b[sort] - a[sort]);
    return arr.slice(0, limit);
  }, [usageRows, sort, limit]);

  if (aggregated.length === 0) {
    return (
      <section className="rounded-xl border border-card-border bg-card p-6 text-center text-sm text-muted">
        No projects yet. Run <code className="font-mono">claude</code> to start tracking.
      </section>
    );
  }

  return (
    <section
      aria-label="Top projects"
      className="rounded-xl border border-card-border bg-card p-4"
    >
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-medium">Top projects</h2>
      </div>
      <table className="w-full text-sm">
        <thead>
          <tr className="text-left text-xs uppercase tracking-wider text-muted">
            <th scope="col" className="px-2 py-2">Project</th>
            <SortableHeader label="Tokens" active={sort === 'tokens'} onClick={() => setSort('tokens')} />
            <SortableHeader label="Cost" active={sort === 'cost'} onClick={() => setSort('cost')} />
            <SortableHeader label="Sessions" active={sort === 'sessions'} onClick={() => setSort('sessions')} />
          </tr>
        </thead>
        <tbody>
          {aggregated.map((p) => (
            <tr key={p.project} className="border-t border-card-border">
              <td className="px-2 py-2 font-medium">{p.project}</td>
              <td className="px-2 py-2 text-right font-mono tabular">{compact.format(p.tokens)}</td>
              <td className="px-2 py-2 text-right font-mono tabular">{usd.format(p.cost)}</td>
              <td className="px-2 py-2 text-right font-mono tabular">{p.sessions}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}

function SortableHeader({
  label,
  active,
  onClick,
}: {
  label: string;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <th scope="col" className="px-2 py-2 text-right">
      <button
        type="button"
        onClick={onClick}
        className={`inline-flex items-center gap-1 ${active ? 'text-foreground' : 'text-muted hover:text-foreground'}`}
      >
        {label}
        {active && <ArrowDown size={12} />}
      </button>
    </th>
  );
}
```

- [ ] **Step 3: Run test**

```bash
pnpm test -- components/top-projects.test.tsx
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/components/top-projects.tsx web/components/top-projects.test.tsx
git commit -m "feat(web): sortable top projects table"
```

---

## Task 14: Build the RecentSessions list (TDD)

**Files:**
- Create: `web/components/recent-sessions.tsx`
- Create: `web/components/recent-sessions.test.tsx`

Most recent 20 sessions across all profiles. Each session is one card row showing profile dot, project, model, day, cost. Sessions are derived from UsageRows by grouping `(profile, project, day)` and treating each row as one "session-bucket" (since v0.1 doesn't surface per-session detail through the API yet).

- [ ] **Step 1: Failing test**

Create `web/components/recent-sessions.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { RecentSessions } from './recent-sessions';
import type { UsageRow } from '@/lib/api';

const rows: UsageRow[] = Array.from({ length: 25 }, (_, i) => ({
  profile: i % 2 === 0 ? 'work' : 'personal',
  project: `proj-${i}`,
  model: 'claude-opus-4-7',
  day: new Date(Date.UTC(2026, 4, 19 - i)).toISOString(),
  usage: { input_tokens: 100 * i, output_tokens: 50, cache_read_tokens: 0, cache_create_tokens: 0 },
  session_count: 1,
  estimated_usd: 0.5,
}));

describe('<RecentSessions>', () => {
  it('renders at most 20 rows', () => {
    render(<RecentSessions usageRows={rows} profiles={[{ name: 'work' }, { name: 'personal' }]} />);
    expect(screen.getAllByTestId('session-row').length).toBeLessThanOrEqual(20);
  });

  it('shows empty state when no rows', () => {
    render(<RecentSessions usageRows={[]} profiles={[]} />);
    expect(screen.getByText(/no sessions yet/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Implementation**

Create `web/components/recent-sessions.tsx`:

```tsx
'use client';

import { useMemo } from 'react';
import type { UsageRow } from '@/lib/api';
import { profileAccent } from '@/lib/profile-color';

export interface RecentSessionsProps {
  usageRows: UsageRow[];
  profiles: { name: string; color?: string }[];
  limit?: number;
}

const usd = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' });
const dayFmt = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric' });

export function RecentSessions({ usageRows, profiles, limit = 20 }: RecentSessionsProps) {
  const sessions = useMemo(() => {
    const sorted = [...usageRows].sort((a, b) => b.day.localeCompare(a.day));
    return sorted.slice(0, limit);
  }, [usageRows, limit]);

  if (sessions.length === 0) {
    return (
      <section className="rounded-xl border border-card-border bg-card p-6 text-center text-sm text-muted">
        No sessions yet. Run <code className="font-mono">claude</code> to start tracking.
      </section>
    );
  }

  return (
    <section
      aria-label="Recent sessions"
      className="rounded-xl border border-card-border bg-card p-4"
    >
      <h2 className="mb-3 text-sm font-medium">Recent sessions</h2>
      <ul className="divide-y divide-card-border">
        {sessions.map((s, i) => {
          const meta = profiles.find((p) => p.name === s.profile) ?? { name: s.profile };
          return (
            <li
              key={`${s.profile}-${s.project}-${s.day}-${i}`}
              data-testid="session-row"
              className="flex items-center justify-between py-2"
            >
              <div className="flex items-center gap-3">
                <span
                  aria-hidden
                  className="h-2 w-2 rounded-full"
                  style={{ background: profileAccent(meta) }}
                />
                <span className="text-sm font-medium">{s.project ?? '—'}</span>
                <span className="text-xs text-muted">{s.profile}</span>
                {s.model && <span className="text-xs text-muted">{s.model}</span>}
              </div>
              <div className="flex items-center gap-4 font-mono text-xs tabular">
                <span className="text-muted">{dayFmt.format(new Date(s.day))}</span>
                <span>{usd.format(s.estimated_usd)}</span>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
```

- [ ] **Step 3: Run test**

```bash
pnpm test -- components/recent-sessions.test.tsx
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/components/recent-sessions.tsx web/components/recent-sessions.test.tsx
git commit -m "feat(web): recent sessions list"
```

---

## Task 15: Build the Footer

**Files:**
- Create: `web/components/footer.tsx`
- Create: `web/components/footer.test.tsx`
- Create: `web/lib/version.ts`

- [ ] **Step 1: Version helper**

Create `web/lib/version.ts`:

```ts
/** Build-time version. Replaced by `NEXT_PUBLIC_CCX_VERSION` env var if set. */
export const CCX_VERSION =
  (typeof process !== 'undefined' && process.env.NEXT_PUBLIC_CCX_VERSION) || '0.1.0-dev';

export const CCX_REPO_URL = 'https://github.com/arafa-dev/ccx';
```

- [ ] **Step 2: Failing test**

Create `web/components/footer.test.tsx`:

```tsx
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Footer } from './footer';

describe('<Footer>', () => {
  it('shows version and github link', () => {
    render(<Footer lastRefreshed={new Date('2026-05-19T12:00:00Z')} />);
    expect(screen.getByText(/v0\.1\.0/i)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /github/i })).toHaveAttribute(
      'href',
      'https://github.com/arafa-dev/ccx',
    );
  });

  it('shows a relative refreshed timestamp', () => {
    render(<Footer lastRefreshed={new Date(Date.now() - 5_000)} />);
    expect(screen.getByText(/refreshed/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Implementation**

Create `web/components/footer.tsx`:

```tsx
'use client';

import { Github } from 'lucide-react';
import { CCX_REPO_URL, CCX_VERSION } from '@/lib/version';

export interface FooterProps {
  lastRefreshed: Date;
}

function relative(date: Date): string {
  const diff = Math.max(0, (Date.now() - date.getTime()) / 1000);
  if (diff < 60) return `${Math.round(diff)}s ago`;
  if (diff < 3600) return `${Math.round(diff / 60)}m ago`;
  return `${Math.round(diff / 3600)}h ago`;
}

export function Footer({ lastRefreshed }: FooterProps) {
  return (
    <footer className="mt-6 flex flex-wrap items-center justify-between gap-3 border-t border-card-border px-6 py-4 text-xs text-muted">
      <div className="flex items-center gap-3">
        <span className="font-mono">ccx v{CCX_VERSION}</span>
        <a
          href={CCX_REPO_URL}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 hover:text-foreground"
          aria-label="ccx on GitHub"
        >
          <Github size={12} />
          GitHub
        </a>
      </div>
      <span>refreshed {relative(lastRefreshed)}</span>
    </footer>
  );
}
```

- [ ] **Step 4: Run test**

```bash
pnpm test -- components/footer.test.tsx
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/components/footer.tsx web/components/footer.test.tsx web/lib/version.ts
git commit -m "feat(web): footer with version, github link, last refreshed"
```

---

## Task 16: Compose the Dashboard page (TDD — picker filtering)

**Files:**
- Create: `web/components/dashboard.tsx`
- Create: `web/components/dashboard.test.tsx`
- Modify: `web/app/page.tsx`

- [ ] **Step 1: Failing test for filtering behavior**

Create `web/components/dashboard.test.tsx`:

```tsx
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Dashboard } from './dashboard';
import { ThemeProvider } from './theme-provider';
import type { ProfileWithTotals, UsageRow, UsageTotal } from '@/lib/api';

const profiles: ProfileWithTotals[] = [
  {
    name: 'work',
    config_dir: '/x',
    color: '#3B82F6',
    created_at: '2026-05-01T00:00:00Z',
    last_used_at: '2026-05-19T00:00:00Z',
    today: blankTotal(),
  },
  {
    name: 'personal',
    config_dir: '/y',
    color: '#10B981',
    created_at: '2026-05-01T00:00:00Z',
    last_used_at: '2026-05-19T00:00:00Z',
    today: blankTotal(),
  },
];

function blankTotal(): UsageTotal {
  return {
    usage: { input_tokens: 0, output_tokens: 0, cache_read_tokens: 0, cache_create_tokens: 0 },
    estimated_usd: 0,
  };
}

function row(profile: string, project: string): UsageRow {
  return {
    profile,
    project,
    day: '2026-05-19T00:00:00Z',
    usage: { input_tokens: 100, output_tokens: 50, cache_read_tokens: 0, cache_create_tokens: 0 },
    session_count: 1,
    estimated_usd: 1,
  };
}

const allRows: UsageRow[] = [row('work', 'acme'), row('personal', 'hobby'), row('work', 'ccx')];

vi.mock('@/lib/api', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api');
  return {
    ...actual,
    getProfiles: vi.fn(async () => profiles),
    getUsage: vi.fn(async ({ profile }: { profile?: string } = {}) => {
      const rows = profile ? allRows.filter((r) => r.profile === profile) : allRows;
      return { rows, total: blankTotal() };
    }),
    getHealth: vi.fn(async () => ({ ok: true, version: '0.1.0-test' })),
    streamUsage: vi.fn(() => () => {}),
  };
});

describe('<Dashboard>', () => {
  beforeEach(() => vi.clearAllMocks());

  it('loads profiles and renders cards', async () => {
    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );
    await waitFor(() => {
      expect(screen.getByText('work')).toBeInTheDocument();
      expect(screen.getByText('personal')).toBeInTheDocument();
    });
  });

  it('filters the projects table when picker changes', async () => {
    render(
      <ThemeProvider>
        <Dashboard />
      </ThemeProvider>,
    );

    await waitFor(() => expect(screen.getByText('acme')).toBeInTheDocument());
    expect(screen.getByText('hobby')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: /filter/i }));
    await userEvent.click(screen.getByRole('menuitem', { name: /^work$/i }));

    await waitFor(() => {
      expect(screen.queryByText('hobby')).not.toBeInTheDocument();
      expect(screen.getByText('acme')).toBeInTheDocument();
    });
  });
});
```

- [ ] **Step 2: Dashboard component**

Create `web/components/dashboard.tsx`:

```tsx
'use client';

import { useCallback, useEffect, useState } from 'react';
import { Header, type LiveStatus } from './header';
import { ProfileCards } from './profile-cards';
import { TimeSeriesChart } from './time-series-chart';
import { TopProjects } from './top-projects';
import { RecentSessions } from './recent-sessions';
import { Footer } from './footer';
import {
  getHealth,
  getProfiles,
  getUsage,
  streamUsage,
  type ProfileWithTotals,
  type UsageRow,
} from '@/lib/api';

export function Dashboard() {
  const [profiles, setProfiles] = useState<ProfileWithTotals[]>([]);
  const [usageRows, setUsageRows] = useState<UsageRow[]>([]);
  const [selectedProfile, setSelectedProfile] = useState<string | null>(null);
  const [live, setLive] = useState<LiveStatus>('connecting');
  const [refreshedAt, setRefreshedAt] = useState<Date>(new Date());
  const [loadError, setLoadError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    try {
      const [p, u] = await Promise.all([
        getProfiles(),
        getUsage({ profile: selectedProfile ?? undefined, since: '7d' }),
      ]);
      setProfiles(p);
      setUsageRows(u.rows);
      setRefreshedAt(new Date());
      setLoadError(null);
    } catch (e) {
      setLoadError(e instanceof Error ? e.message : String(e));
    }
  }, [selectedProfile]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    let cancelled = false;
    void getHealth()
      .then(() => {
        if (!cancelled) setLive('connected');
      })
      .catch(() => {
        if (!cancelled) setLive('disconnected');
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const stop = streamUsage(
      () => {
        setRefreshedAt(new Date());
        setLive('connected');
        void refresh();
      },
      () => setLive('disconnected'),
    );
    return stop;
  }, [refresh]);

  return (
    <div className="mx-auto flex min-h-screen max-w-7xl flex-col">
      <Header
        profiles={profiles.map((p) => ({ name: p.name, color: p.color }))}
        selected={selectedProfile}
        onSelect={setSelectedProfile}
        live={live}
      />

      <main className="flex flex-col gap-6 px-6 py-6">
        {loadError && (
          <div className="rounded-lg border border-red-500/40 bg-red-500/10 px-4 py-3 text-sm">
            Failed to load: {loadError}
          </div>
        )}

        {profiles.length === 0 && !loadError ? (
          <OnboardingEmpty />
        ) : (
          <>
            <ProfileCards
              profiles={
                selectedProfile ? profiles.filter((p) => p.name === selectedProfile) : profiles
              }
              usageRows={usageRows}
            />
            <TimeSeriesChart
              usageRows={usageRows}
              profiles={profiles.map((p) => ({ name: p.name, color: p.color }))}
            />
            <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
              <TopProjects usageRows={usageRows} />
              <RecentSessions
                usageRows={usageRows}
                profiles={profiles.map((p) => ({ name: p.name, color: p.color }))}
              />
            </div>
          </>
        )}
      </main>

      <Footer lastRefreshed={refreshedAt} />
    </div>
  );
}

function OnboardingEmpty() {
  return (
    <section className="rounded-xl border border-card-border bg-card p-8 text-center">
      <h2 className="text-lg font-medium">No profiles yet</h2>
      <p className="mt-2 text-sm text-muted">
        Register your first Claude Code account to start tracking usage:
      </p>
      <pre className="mt-4 inline-block rounded-md bg-grid px-4 py-2 text-left font-mono text-xs">
        ccx profile add work --config-dir ~/.claude-profiles/work
      </pre>
    </section>
  );
}
```

- [ ] **Step 3: Wire `web/app/page.tsx`**

Replace contents of `web/app/page.tsx`:

```tsx
import { Dashboard } from '@/components/dashboard';

export default function HomePage() {
  return <Dashboard />;
}
```

- [ ] **Step 4: Run the tests**

```bash
pnpm test
```

Expected: every previously-added test passes; the new `dashboard.test.tsx` passes.

- [ ] **Step 5: Commit**

```bash
git add web/components/dashboard.tsx web/components/dashboard.test.tsx web/app/page.tsx
git commit -m "feat(web): compose dashboard layout and wire data fetching"
```

---

## Task 17: Add MSW handlers test (verify mock surface matches the contract)

**Files:**
- Create: `web/mocks/handlers.test.ts`

This is a guard rail: the handlers must respond to every endpoint defined in `api/openapi.yaml`.

- [ ] **Step 1: Test**

Create `web/mocks/handlers.test.ts`:

```ts
import { describe, it, expect, beforeAll, afterAll, afterEach } from 'vitest';
import { setupServer } from 'msw/node';
import { handlers } from './handlers';

const server = setupServer(...handlers);

beforeAll(() => server.listen({ onUnhandledRequest: 'error' }));
afterEach(() => server.resetHandlers());
afterAll(() => server.close());

const base = (process.env.NEXT_PUBLIC_API_BASE ?? 'http://127.0.0.1:7777').replace(/\/$/, '');

describe('MSW handlers cover every openapi.yaml endpoint', () => {
  it('GET /api/health', async () => {
    const res = await fetch(`${base}/api/health`);
    expect(res.status).toBe(200);
    const body = (await res.json()) as { ok: boolean; version: string };
    expect(body.ok).toBe(true);
    expect(typeof body.version).toBe('string');
  });

  it('GET /api/profiles', async () => {
    const res = await fetch(`${base}/api/profiles`);
    expect(res.status).toBe(200);
    const body = (await res.json()) as unknown[];
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThanOrEqual(3);
  });

  it('GET /api/usage with no filter', async () => {
    const res = await fetch(`${base}/api/usage`);
    expect(res.status).toBe(200);
    const body = (await res.json()) as { rows: unknown[]; total: unknown };
    expect(Array.isArray(body.rows)).toBe(true);
    expect(body.total).toBeDefined();
  });

  it('GET /api/usage filters by profile', async () => {
    const res = await fetch(`${base}/api/usage?profile=work`);
    expect(res.status).toBe(200);
    const body = (await res.json()) as { rows: { profile: string }[] };
    for (const r of body.rows) expect(r.profile).toBe('work');
  });

  it('GET /api/usage/live serves text/event-stream', async () => {
    const res = await fetch(`${base}/api/usage/live`);
    expect(res.status).toBe(200);
    expect(res.headers.get('content-type')).toContain('text/event-stream');
    const text = await res.text();
    expect(text).toContain('event: usage');
  });
});
```

- [ ] **Step 2: Run**

```bash
pnpm test -- mocks/handlers.test.ts
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add web/mocks/handlers.test.ts
git commit -m "test(web): verify MSW handlers cover every openapi endpoint"
```

---

## Task 18: ESLint config + lint clean

**Files:**
- Modify: `web/.eslintrc.json` (already created by create-next-app)
- Verify: clean lint pass

- [ ] **Step 1: Inspect and tighten the eslint config**

Open `web/.eslintrc.json`. Ensure contents look like:

```json
{
  "extends": ["next/core-web-vitals", "next/typescript"],
  "rules": {
    "@typescript-eslint/no-unused-vars": ["error", { "argsIgnorePattern": "^_", "varsIgnorePattern": "^_" }],
    "@typescript-eslint/consistent-type-imports": "error"
  }
}
```

If the file is `eslint.config.mjs` (flat config — Next 15 may emit either), translate the rules into that format. Either form is acceptable; what matters is `pnpm lint` exits 0.

- [ ] **Step 2: Run**

```bash
pnpm lint
pnpm typecheck
```

Expected: both exit 0. Fix any warnings before continuing.

- [ ] **Step 3: Commit (if eslint config changed)**

```bash
git add web/.eslintrc.json
git commit -m "chore(web): tighten eslint rules"
```

If nothing changed, skip the commit.

---

## Task 19: Playwright E2E setup + happy-path test

**Files:**
- Create: `web/playwright.config.ts`
- Create: `web/e2e/dashboard.spec.ts`

- [ ] **Step 1: Install browsers**

```bash
cd web && pnpm e2e:install
```

- [ ] **Step 2: Playwright config**

Create `web/playwright.config.ts`:

```ts
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: 'list',
  use: {
    baseURL: 'http://127.0.0.1:3001',
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  webServer: {
    command: 'pnpm dev',
    port: 3001,
    reuseExistingServer: !process.env.CI,
    timeout: 60_000,
  },
});
```

- [ ] **Step 3: Happy-path E2E test**

Create `web/e2e/dashboard.spec.ts`:

```ts
import { test, expect } from '@playwright/test';

test('dashboard loads with three mocked profiles and no console errors', async ({ page }) => {
  const consoleErrors: string[] = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error') consoleErrors.push(msg.text());
  });

  await page.goto('/');

  // ccx wordmark
  await expect(page.getByText(/^ccx$/)).toBeVisible();

  // Three profile cards (from MSW fixtures)
  await expect(page.getByTestId('profile-card')).toHaveCount(3);

  // Chart is in the DOM
  await expect(page.getByTestId('time-series-chart')).toBeVisible();

  // Top projects section
  await expect(page.getByText(/top projects/i)).toBeVisible();

  // Recent sessions section
  await expect(page.getByText(/recent sessions/i)).toBeVisible();

  // No console errors (MSW boots and fixtures serve cleanly)
  expect(consoleErrors).toEqual([]);
});

test('profile picker filters projects', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByTestId('profile-card')).toHaveCount(3);

  await page.getByRole('button', { name: /filter/i }).click();
  await page.getByRole('menuitem', { name: /^work$/ }).click();

  // After selecting "work", only one profile card visible
  await expect(page.getByTestId('profile-card')).toHaveCount(1);
});
```

- [ ] **Step 4: Run the E2E**

```bash
pnpm e2e
```

Expected: both tests PASS. If Playwright complains about missing browsers, run `pnpm e2e:install` again.

- [ ] **Step 5: Commit**

```bash
git add web/playwright.config.ts web/e2e/
git commit -m "test(web): playwright e2e happy-path"
```

---

## Task 20: Production build verification

**Files:**
- No new files; verify build output

- [ ] **Step 1: Clean any stale artifacts**

```bash
cd web && rm -rf .next out
```

- [ ] **Step 2: Production build**

```bash
NODE_ENV=production pnpm build
```

Expected: exit 0; produces `web/out/index.html`, `web/out/_next/`, and a `web/out/mockServiceWorker.js` (the MSW worker file is in `public/` and therefore copied to `out/`, which is harmless in production — the dev-only `MswBoot` component does not register it).

- [ ] **Step 3: Inspect the export**

```bash
test -f out/index.html && echo "index.html exists"
test -d out/_next && echo "_next assets exist"
ls -la out/ | head -20
```

Expected: `index.html` present; `_next` directory present.

- [ ] **Step 4: Smoke-serve and verify**

```bash
pnpm exec serve out -p 3002 &
SERVE_PID=$!
sleep 2
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:3002/
kill $SERVE_PID
```

Expected: HTTP 200.

- [ ] **Step 5: Commit (no new files; this is a verification task — skip commit if clean)**

If any docs/scripts were updated during smoke testing, commit them. Otherwise no commit.

---

## Task 21: Add npm ecosystem to dependabot

**Files:**
- Modify: `.github/dependabot.yml` (created in Phase 0)

- [ ] **Step 1: Append the npm block**

Append to `.github/dependabot.yml`:

```yaml
  - package-ecosystem: "npm"
    directory: "/web"
    schedule:
      interval: "weekly"
    open-pull-requests-limit: 5
    commit-message:
      prefix: "build"
    groups:
      next-stack:
        patterns:
          - "next"
          - "react"
          - "react-dom"
          - "@types/react*"
      test-stack:
        patterns:
          - "vitest"
          - "@testing-library/*"
          - "@playwright/test"
          - "playwright"
          - "jsdom"
          - "msw"
```

- [ ] **Step 2: Commit**

```bash
git add .github/dependabot.yml
git commit -m "ci: enable dependabot for web/"
```

---

## Task 22: Add web CI job

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Add a `web` job that lints, typechecks, tests, builds, and runs e2e**

Append to `.github/workflows/ci.yml` after the existing `build` job (under the same top-level `jobs:` key):

```yaml
  web:
    name: Web (lint + test + build)
    runs-on: ubuntu-22.04
    defaults:
      run:
        working-directory: web
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with:
          version: 9
      - uses: actions/setup-node@v4
        with:
          node-version: "20"
          cache: "pnpm"
          cache-dependency-path: web/pnpm-lock.yaml
      - name: Install deps
        run: pnpm install --frozen-lockfile
      - name: Lint
        run: pnpm lint
      - name: Typecheck
        run: pnpm typecheck
      - name: Check api types are in sync
        run: pnpm check:api-types
      - name: Unit tests
        run: pnpm test
      - name: Build
        run: pnpm build
      - name: Verify static export
        run: |
          test -f out/index.html
          test -d out/_next
      - name: Install Playwright
        run: pnpm exec playwright install --with-deps chromium
      - name: E2E
        run: pnpm e2e
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add web lint+test+build+e2e job"
```

---

## Task 23: Final local CI gate + Lighthouse check

- [ ] **Step 1: From `web/`, run the whole pipeline**

```bash
cd web
pnpm lint
pnpm typecheck
pnpm check:api-types
pnpm test
pnpm build
test -f out/index.html
pnpm e2e
```

Expected: every command exits 0.

- [ ] **Step 2: Manual Lighthouse check**

```bash
pnpm exec serve out -p 3002 &
SERVE_PID=$!
sleep 2
```

Open `http://127.0.0.1:3002` in Chrome → DevTools → Lighthouse → Mobile → "Performance" and "Accessibility" categories → Analyze page load.

Record the scores. **Required:** Performance ≥ 90 AND Accessibility ≥ 95. If either is below the threshold, investigate (typically: image sizes, contrast, missing aria-labels, font preload). Fix and re-run.

```bash
kill $SERVE_PID
```

- [ ] **Step 3: Document the scores**

Append to `web/README.md`:

```markdown

## Performance baseline (Lighthouse, mobile config)

| Run | Performance | Accessibility | Best Practices | SEO |
|---|---|---|---|---|
| Initial baseline | <fill in your score> | <fill in your score> | — | — |

Re-run after any major UI change. Performance must stay ≥ 90, Accessibility ≥ 95.
```

Replace `<fill in your score>` with actual measured numbers (e.g. `94`, `97`).

- [ ] **Step 4: Commit**

```bash
git add web/README.md
git commit -m "docs(web): record Lighthouse baseline"
```

---

## Task 24: Push and open PR

- [ ] **Step 1: Push the feature branch**

```bash
git push -u origin feat/web
```

- [ ] **Step 2: Open the PR**

```bash
gh pr create \
  --base main \
  --head feat/web \
  --title "feat(web): Next.js dashboard (Phase 1 A7)" \
  --body "$(cat <<'EOF'
## What

Implements Phase 1 plan A7 — the Next.js 15 static-export dashboard. Six layout
sections (header, profile cards, time-series chart, top projects, recent sessions,
footer) backed by MSW fixtures during Phase 1. The Go server will replace MSW
in Phase 2 via the OpenAPI contract.

## Why

The dashboard is the most-screenshotted artifact of ccx and the only way to
view aggregated usage across multiple profiles in one place. Building it
independently of the Go backend (via MSW) lets it ship in parallel with the
A1–A6 Go packages.

## Contract impact

- [x] This PR does NOT modify `internal/contracts/`, `api/openapi.yaml`,
      `internal/storage/schema.sql`, or `docs/conventions.md`

## Checklist

- [x] Tests added/updated and all pass locally (`pnpm test`, `pnpm e2e`)
- [x] Lint clean locally (`pnpm lint`)
- [x] No new dependencies without justification
- [x] Lighthouse: Performance ≥ 90, Accessibility ≥ 95

## Phase 1 worktree

- Package: `web/`
- Plan: `docs/superpowers/plans/2026-05-19-ccx-phase-1-A7-web.md`
EOF
)"
```

- [ ] **Step 3: Watch CI**

```bash
gh pr checks --watch
```

Expected: lint, test (3 OS), build (3 OS), and `web` job all green.

If anything fails, fix locally, push a new commit, re-watch. Never bypass branch protection.

---

## Phase 1 A7 done definition

All of the following are true:

- [ ] `pnpm --filter web build` (or `pnpm build` from `web/`) produces `web/out/index.html` and `web/out/_next/`
- [ ] `pnpm --filter web dev` serves the dashboard at `http://localhost:3001` with MSW returning realistic data for all endpoints in `api/openapi.yaml`
- [ ] `pnpm test` passes (api client, profile-color, header, profile-cards, time-series-chart, top-projects, recent-sessions, footer, dashboard, MSW handlers)
- [ ] `pnpm e2e` passes
- [ ] `pnpm lint` and `pnpm typecheck` exit 0
- [ ] `pnpm check:api-types` exits 0 (committed `lib/api-types.ts` matches `api/openapi.yaml`)
- [ ] Dark mode default; toggle works; profile picker filters the chart, projects table, and cards
- [ ] All six layout sections render with realistic data: Header, ProfileCards, TimeSeriesChart, TopProjects, RecentSessions, Footer
- [ ] Profile accent color is deterministic and consistent across cards, picker dots, sparkline, and stacked chart bands
- [ ] Production preview load time under 1s on a modern laptop
- [ ] Lighthouse mobile config: Performance ≥ 90 AND Accessibility ≥ 95 (numbers recorded in `web/README.md`)
- [ ] CI workflow `web` job green on the PR
- [ ] PR opened against `main`, awaiting review

After this plan merges, downstream Phase 2 integration (P2) will replace MSW with the real Go server. No `web/` file should need to change at that point — the API client already targets `process.env.NEXT_PUBLIC_API_BASE`, and the static export is consumed via `//go:embed all:web/out` in `internal/dashboard/`.
