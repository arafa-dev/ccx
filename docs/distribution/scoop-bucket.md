# Scoop bucket setup - `arafa-dev/scoop-ccx`

This is the bootstrap process for the Scoop bucket that hosts the `ccx`
manifest. Once set up, **goreleaser updates the manifest automatically on
every release**.

## 1. Create the GitHub repo

Manually create a new public repo on GitHub:

- Owner: `arafa-dev`
- Name: `scoop-ccx`
- Description: "Scoop bucket for ccx - multi-account workspace manager for Claude Code"
- Initialize with: nothing

## 2. Clone and bootstrap locally

```bash
git clone git@github.com:arafa-dev/scoop-ccx.git
cd scoop-ccx
mkdir -p bucket
```

## 3. Add a bootstrap `README.md`

````markdown
# scoop-ccx

Scoop bucket for [ccx](https://github.com/arafa-dev/ccx) - multi-account
workspace manager for Claude Code.

## Install

```powershell
scoop bucket add ccx https://github.com/arafa-dev/scoop-ccx
scoop install ccx
```

This bucket is **updated automatically** by goreleaser on every ccx release.
Do not edit `bucket/ccx.json` by hand - your changes will be overwritten on
the next release.
````

## 4. Add a bootstrap `bucket/ccx.json`

A minimum valid Scoop manifest so `scoop install` does not fail before the
first goreleaser run. The next release overwrites it with real version + URLs.

```json
{
  "version": "0.0.0",
  "description": "Multi-account workspace manager for Claude Code",
  "homepage": "https://github.com/arafa-dev/ccx",
  "license": "MIT",
  "architecture": {
    "64bit": {
      "url": "https://github.com/arafa-dev/ccx/releases/download/v0.0.0/ccx_0.0.0_windows_amd64.zip",
      "hash": "0000000000000000000000000000000000000000000000000000000000000000"
    },
    "arm64": {
      "url": "https://github.com/arafa-dev/ccx/releases/download/v0.0.0/ccx_0.0.0_windows_arm64.zip",
      "hash": "0000000000000000000000000000000000000000000000000000000000000000"
    }
  },
  "bin": "ccx.exe",
  "checkver": {
    "github": "https://github.com/arafa-dev/ccx"
  },
  "autoupdate": {
    "architecture": {
      "64bit": { "url": "https://github.com/arafa-dev/ccx/releases/download/v$version/ccx_$version_windows_amd64.zip" },
      "arm64": { "url": "https://github.com/arafa-dev/ccx/releases/download/v$version/ccx_$version_windows_arm64.zip" }
    }
  }
}
```

## 5. Commit and push

```bash
git add README.md bucket/ccx.json
git commit -m "chore: bootstrap scoop-ccx bucket"
git push origin main
```

## 6. Reuse the same `GH_PAT` secret

The `GH_PAT` configured in the Homebrew tap setup (see
`docs/distribution/homebrew-tap.md`) already includes `arafa-dev/scoop-ccx`
in its repo scope. No separate secret is needed.

## 7. Verify the bucket works

```powershell
scoop bucket add ccx https://github.com/arafa-dev/scoop-ccx
scoop search ccx
# Should list ccx (from arafa-dev_scoop-ccx)
```

Installing the bootstrap version will fail (the 0.0.0 archives don't exist),
which is expected. The first real `v0.1.0` release will populate the manifest
with real URLs and hashes automatically.
