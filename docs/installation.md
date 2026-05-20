# Installing ccx

## macOS

```bash
brew install arafa-dev/ccx/ccx
```

Requires Homebrew. After install, set up shell integration once:

```bash
echo 'eval "$(ccx init zsh)"' >> ~/.zshrc   # or ~/.bashrc for bash
source ~/.zshrc
```

## Linux

### Debian / Ubuntu

```bash
curl -fsSL https://github.com/arafa-dev/ccx/releases/latest/download/ccx_linux_amd64.deb -o /tmp/ccx.deb
sudo dpkg -i /tmp/ccx.deb
```

For arm64: replace `amd64` with `arm64`.

### Fedora / RHEL

```bash
sudo rpm -i https://github.com/arafa-dev/ccx/releases/latest/download/ccx_linux_amd64.rpm
```

### Other distros (binary)

```bash
curl -fsSL https://raw.githubusercontent.com/arafa-dev/ccx/main/install.sh | sh
```

The script picks the right binary for your OS and arch, verifies its SHA256, and installs it to `/usr/local/bin/ccx` (or `~/.local/bin/ccx` if not root).

Then set up shell integration:

```bash
echo 'eval "$(ccx init bash)"' >> ~/.bashrc    # or ~/.zshrc for zsh
echo 'ccx init fish | source' >> ~/.config/fish/config.fish   # for fish
source ~/.bashrc
```

## Windows

```powershell
scoop bucket add ccx https://github.com/arafa-dev/scoop-ccx
scoop install ccx
```

Then add this to your PowerShell profile (`$PROFILE`):

```powershell
ccx init pwsh | Out-String | Invoke-Expression
```

## From source

Requires Go 1.22+:

```bash
go install github.com/arafa-dev/ccx/cmd/ccx@latest
```

## Verifying the install

```bash
ccx version
ccx doctor
```

`ccx doctor` reports the status of your install and any registered profiles. Every check should show ✅ for a healthy setup.

## Uninstalling

```bash
brew uninstall ccx                              # macOS
sudo apt remove ccx                             # Debian/Ubuntu
sudo dnf remove ccx                             # Fedora
scoop uninstall ccx                             # Windows
rm /usr/local/bin/ccx                           # binary install
rm -rf ~/.ccx                                   # state directory
```
