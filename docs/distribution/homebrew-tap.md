# Homebrew tap setup - `arafa-dev/homebrew-ccx`

This is the bootstrap process for the Homebrew tap that hosts the `ccx` formula.
Once set up, **goreleaser updates the formula automatically on every release** -
you never edit `Formula/ccx.rb` by hand after the bootstrap.

## 1. Create the GitHub repo

Manually create a new public repo on GitHub:

- Owner: `arafa-dev`
- Name: `homebrew-ccx` (the `homebrew-` prefix is required by Homebrew)
- Description: "Homebrew tap for ccx - multi-account workspace manager for Claude Code"
- Initialize with: nothing (no README, no .gitignore, no license - we add them below)

## 2. Clone and bootstrap locally

```bash
git clone git@github.com:arafa-dev/homebrew-ccx.git
cd homebrew-ccx
mkdir -p Formula
```

## 3. Add a bootstrap `README.md`

````markdown
# homebrew-ccx

Homebrew tap for [ccx](https://github.com/arafa-dev/ccx) - multi-account
workspace manager for Claude Code.

## Install

```bash
brew install arafa-dev/ccx/ccx
```

This tap is **updated automatically** by goreleaser on every ccx release.
Do not edit `Formula/ccx.rb` by hand - your changes will be overwritten on
the next release.
````

## 4. Add a bootstrap `Formula/ccx.rb`

This file just needs to be a valid Homebrew formula so `brew tap` succeeds
before the first goreleaser run. Goreleaser overwrites it on the next release.

```ruby
class Ccx < Formula
  desc "Multi-account workspace manager for Claude Code"
  homepage "https://github.com/arafa-dev/ccx"
  version "0.0.0"
  license "MIT"

  on_macos do
    on_intel { url "https://github.com/arafa-dev/ccx/releases/download/v0.0.0/ccx_0.0.0_darwin_amd64.tar.gz"; sha256 "0000000000000000000000000000000000000000000000000000000000000000" }
    on_arm { url "https://github.com/arafa-dev/ccx/releases/download/v0.0.0/ccx_0.0.0_darwin_arm64.tar.gz"; sha256 "0000000000000000000000000000000000000000000000000000000000000000" }
  end

  on_linux do
    on_intel { url "https://github.com/arafa-dev/ccx/releases/download/v0.0.0/ccx_0.0.0_linux_amd64.tar.gz"; sha256 "0000000000000000000000000000000000000000000000000000000000000000" }
    on_arm { url "https://github.com/arafa-dev/ccx/releases/download/v0.0.0/ccx_0.0.0_linux_arm64.tar.gz"; sha256 "0000000000000000000000000000000000000000000000000000000000000000" }
  end

  def install
    bin.install "ccx"
  end

  test do
    system "#{bin}/ccx", "version"
  end
end
```

## 5. Commit and push

```bash
git add README.md Formula/ccx.rb
git commit -m "chore: bootstrap homebrew-ccx tap"
git push origin main
```

## 6. Grant goreleaser permission to push

The ccx release workflow uses the `GH_PAT` secret to push commits to this tap.
Create a fine-grained personal access token:

- Go to GitHub -> Settings -> Developer settings -> Personal access tokens -> Fine-grained tokens
- Name: `ccx-release-tap-push`
- Resource owner: `arafa-dev`
- Repository access: select **`arafa-dev/homebrew-ccx` AND `arafa-dev/scoop-ccx`**
  (one token for both)
- Permissions -> Repository -> `Contents`: **Read and write**
- Permissions -> Repository -> `Metadata`: Read-only (auto)
- Expiry: 1 year

Copy the token. In the `arafa-dev/ccx` repo:

- Settings -> Secrets and variables -> Actions -> New repository secret
- Name: `GH_PAT`
- Value: paste the token

## 7. Verify the tap works

```bash
brew tap arafa-dev/ccx
brew search ccx
# Should list arafa-dev/ccx/ccx
```

Installing the bootstrap version will fail (the 0.0.0 archives don't exist),
which is expected. The first real `v0.1.0` release will populate the formula
with real URLs and sha256 values automatically.
