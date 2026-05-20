# Diagram assets

This directory holds non-code assets referenced from documentation.

## `architecture.png` and `architecture.svg`

Hand-authored in Excalidraw. Source: `architecture.excalidraw` (commit it alongside).

### What the diagram shows

Three lanes, left-to-right:

1. **User terminal**
   - Box: `$ ccx use work`
   - Arrow → ccx CLI box

2. **ccx CLI (a single box, slightly larger)**
   - Inside: list of subcomponents — `profile`, `scanner`, `storage`, `pricing`, `shell`, `server`, `tui`, `doctor`
   - Arrow down → SQLite cylinder (`~/.ccx/state.db`)
   - Arrow up → Browser box (`localhost:7777`)

3. **Upstream claude (a separate box)**
   - Box: `claude` CLI
   - Arrow up to `Anthropic API` cloud (greyed)
   - Annotation: `CLAUDE_CONFIG_DIR` arrow connecting ccx CLI → claude CLI

### Annotations

- Label the arrow from ccx to claude with `CLAUDE_CONFIG_DIR` (this is the whole switching mechanism)
- Label the SQLite cylinder as `incremental cache (rebuilt from JSONL)`
- Label the JSONL source: `~/.claude*/projects/<cwd>/<uuid>.jsonl`

### Export settings

- PNG: 2400×1350 (2x retina), white background
- SVG: same dimensions, transparent background

After exporting, place both at `docs/assets/architecture.png` and
`docs/assets/architecture.svg`. README references the PNG.

## `dashboard.png`

Screenshot of the dashboard in dark mode, taken from a 1440×900 viewport with
realistic data. Captured during Phase 3 (Polish) after the dashboard is
functional.

## `demo.gif`

Rendered from `demo.tape` via `vhs docs/assets/demo.tape`. Rendered fresh during Phase 3 with the working binary.
