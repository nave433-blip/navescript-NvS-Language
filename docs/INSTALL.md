# Installing NvS

Three ways to get the `nvs` binary. Pick whichever fits.

## 1. The installer (easiest)

```sh
curl -fsSL https://raw.githubusercontent.com/nave433-blip/navescript-NvS-Language/master/install.sh | sh
```

What it does:

1. Detects your OS and CPU (Linux/macOS/Windows, amd64/arm64).
2. Tries a prebuilt release tarball from GitHub Releases
   (`releases/download/<tag>/nvs-<os>-<arch>.tar.gz`).
3. **Falls back to building from source** with your Go toolchain if no
   prebuilt asset exists — which is the case today, because **no
   releases have been published yet**. The installer says so out loud
   instead of failing mysteriously.
4. Copies the binary to `~/.local/bin` (override with
   `NVS_PREFIX=/somewhere` or `--prefix=/somewhere`).

Options as environment variables:

```sh
NVS_VERSION=v0.2.0 sh install.sh        # a specific tag (falls back to source)
NVS_PREFIX=/usr/local sh install.sh    # system-wide (needs write permission)
sh install.sh --dry-run                # print every step, change nothing
```

After installing, make sure `~/.local/bin` is on your `PATH`:

```sh
export PATH="$HOME/.local/bin:$PATH"   # add to ~/.bashrc / ~/.zshrc too
nvs eval 'print "hello, nave"'
```

## 2. Build from source

Needs Go 1.21+.

```sh
git clone https://github.com/nave433-blip/navescript-NvS-Language
cd navescript-NvS-Language
go build -o ~/.local/bin/nvs ./cmd/nvs
```

## 3. Prebuilt tarballs (when releases exist)

When releases are published, each one ships
`nvs-<os>-<arch>.tar.gz` assets (e.g. `nvs-linux-amd64.tar.gz`,
`nvs-darwin-arm64.tar.gz`). Download the one for your platform,
unpack it, and put `nvs` somewhere on your `PATH`:

```sh
tar -xzf nvs-linux-amd64.tar.gz
mv nvs ~/.local/bin/
```

> **Honest status:** as of this writing no release assets exist yet, so
> methods 1 and 2 both build from source. The installer and the asset
> naming above are the contract future releases will follow.

## Windows notes

- The installer runs under Git Bash / MSYS2 / WSL. Native `cmd.exe`
  isn't supported by the shell script — use WSL or build with Go:
  `go build -o nvs.exe ./cmd/nvs`.
- Prebuilt assets for Windows will be named `nvs-windows-amd64.tar.gz`
  (containing `nvs.exe`).

## Uninstall

```sh
rm ~/.local/bin/nvs        # plus ~/.nvs (package cache) if you used nvs pkg
```

## Verifying the install

```sh
nvs version
nvs eval 'print "hello, nave"'
```

Both should print without errors. If `nvs` isn't found, your `PATH`
is missing `~/.local/bin` — see step 1.
