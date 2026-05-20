# Release signing with cosign

Every ccx release signs the `checksums.txt` artifact with [cosign](https://github.com/sigstore/cosign).
Users and CI verify the checksums file before trusting any binary in the release.

## 1. Generate the signing keypair (one time)

On a trusted local machine:

```bash
# Install cosign if not already present
brew install cosign
# or: go install github.com/sigstore/cosign/v2/cmd/cosign@latest

mkdir -p ~/secrets/ccx-cosign
cd ~/secrets/ccx-cosign

cosign generate-key-pair
# Prompts twice for a password. Pick a strong one. The password is needed by
# the release workflow - store it in 1Password / your password manager.
# This creates cosign.key (PRIVATE - never commit) and cosign.pub (public).
```

## 2. Upload the keypair as GitHub secrets

In the `arafa-dev/ccx` repo -> Settings -> Secrets and variables -> Actions:

| Secret name          | Value                                         |
| -------------------- | --------------------------------------------- |
| `COSIGN_PRIVATE_KEY` | Full contents of `cosign.key` (multi-line OK) |
| `COSIGN_PASSWORD`    | The password you set during keypair generation |

## 3. Publish the public key

Two options. Pick **one** (or both) - option B is recommended.

**Option A:** Commit `cosign.pub` to the ccx repo at `docs/distribution/cosign.pub`.
End-users verify with:

```bash
cosign verify-blob \
  --key https://raw.githubusercontent.com/arafa-dev/ccx/main/docs/distribution/cosign.pub \
  --signature checksums.txt.sig \
  checksums.txt
```

**Option B:** Upload `cosign.pub` as a release asset on every release. The
release workflow's verification step (Task 4) will pick it up automatically.
The simplest way is to add it as a constant `extra_files` entry in
`.goreleaser.yaml`:

```yaml
release:
  extra_files:
    - glob: ./docs/distribution/cosign.pub
```

The doc recommends doing **both** - committing the public key to the repo
*and* attaching it as a release asset.

## 4. Local verification by users

After downloading a release, users can verify the integrity:

```bash
TAG=v0.1.0
curl -fsSLO "https://github.com/arafa-dev/ccx/releases/download/$TAG/checksums.txt"
curl -fsSLO "https://github.com/arafa-dev/ccx/releases/download/$TAG/checksums.txt.sig"
curl -fsSLO "https://raw.githubusercontent.com/arafa-dev/ccx/main/docs/distribution/cosign.pub"

cosign verify-blob \
  --key cosign.pub \
  --signature checksums.txt.sig \
  checksums.txt
# -> Verified OK
```

Then verify each binary's sha256 matches the corresponding line in `checksums.txt`.

## 5. Key rotation

If the key is compromised:

1. Generate a new keypair (`cosign generate-key-pair`).
2. Update `COSIGN_PRIVATE_KEY` and `COSIGN_PASSWORD` in repo secrets.
3. Commit the new `cosign.pub`.
4. Add a `SECURITY.md` advisory noting which versions were signed by the old
   key and the date of rotation.
