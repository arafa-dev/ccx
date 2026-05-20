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
