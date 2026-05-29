// Package plandetect resolves a profile's Anthropic plan tier from the local
// Claude Code config (<config_dir>/.claude.json) instead of manual setup. It
// is a leaf package: it reads a file and maps a string, with no sibling
// imports.
package plandetect
