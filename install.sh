#!/bin/sh
# NvS installer — friendly to `curl | sh`.
#
#   curl -fsSL https://raw.githubusercontent.com/nave433-blip/navescript-NvS-Language/master/install.sh | sh
#
# What it does, in order:
#   1. Detects OS and architecture.
#   2. Tries a prebuilt release tarball from GitHub releases:
#        https://github.com/nave433-blip/navescript-NvS-Language/releases/download/<tag>/nvs-<os>-<arch>.tar.gz
#      (No releases are published yet, so today this step 404s and the
#      script says so honestly, then moves on.)
#   3. Falls back to building from source with Go (clone + `go build`).
#   4. Installs the `nvs` binary into $NVS_PREFIX/bin (default ~/.local/bin).
#
# Options:
#   NVS_VERSION=x.y.z   install a specific version (default: latest release,
#                       else the default branch)
#   NVS_PREFIX=/path    install prefix (default: $HOME/.local)
#   sh install.sh --dry-run   print what would happen, change nothing
#
# Exit codes: 0 = installed, 1 = failed (message on stderr).

set -eu

VERSION="${NVS_VERSION:-latest}"
PREFIX="${NVS_PREFIX:-$HOME/.local}"
DRYRUN=0
for arg in "$@"; do
	case "$arg" in
	--dry-run) DRYRUN=1 ;;
	--prefix=*) PREFIX="${arg#--prefix=}" ;;
	-h | --help)
		sed -n '2,24p' "$0"
		exit 0
		;;
	*)
		echo "install.sh: unknown option: $arg" >&2
		exit 1
		;;
	esac
done

REPO_OWNER="nave433-blip"
REPO_NAME="navescript-NvS-Language"
REPO_URL="https://github.com/${REPO_OWNER}/${REPO_NAME}"
RAW_URL="https://raw.githubusercontent.com/${REPO_OWNER}/${REPO_NAME}"

log() { printf '%s\n' "$*"; }
err() { printf 'install.sh: %s\n' "$*" >&2; }
run() {
	if [ "$DRYRUN" -eq 1 ]; then
		log "dry-run: $*"
	else
		"$@"
	fi
}

# ---------------------------------------------------------------------------
# 1. Detect OS / architecture.
# ---------------------------------------------------------------------------
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$OS" in
linux) OS="linux" ;;
darwin) OS="darwin" ;;
mingw* | msys* | cygwin* | windows*) OS="windows" ;;
*) err "unsupported OS: $(uname -s)" && exit 1 ;;
esac
case "$ARCH" in
x86_64 | amd64) ARCH="amd64" ;;
aarch64 | arm64) ARCH="arm64" ;;
*)
	err "unsupported architecture: $(uname -m)"
	exit 1
	;;
esac
EXE=""
[ "$OS" = "windows" ] && EXE=".exe"
log "Detected: ${OS}/${ARCH}"

# ---------------------------------------------------------------------------
# 2. Try a prebuilt release.
# ---------------------------------------------------------------------------
have_cmd() { command -v "$1" >/dev/null 2>&1; }

download() {
	# download <url> <dest> — curl preferred, wget fallback.
	if have_cmd curl; then
		curl -fsSL -o "$2" "$1"
	elif have_cmd wget; then
		wget -q -O "$2" "$1"
	else
		return 1
	fi
}

TMPDIR_WORK="${TMPDIR:-/tmp}/nvs-install-$$"
run mkdir -p "$TMPDIR_WORK"
trap 'rm -rf "$TMPDIR_WORK"' EXIT INT TERM

INSTALLED=""
if [ "$VERSION" = "latest" ]; then
	# Resolve the latest release tag via the GitHub API (unauthenticated).
	LATEST_TAG=""
	if have_cmd curl; then
		LATEST_TAG="$(curl -fsSL "https://api.github.com/repos/${REPO_OWNER}/${REPO_NAME}/releases/latest" 2>/dev/null |
			sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1 || true)"
	fi
	if [ -n "$LATEST_TAG" ]; then
		VERSION="$LATEST_TAG"
		log "Latest release: $VERSION"
	else
		log "No releases found via the GitHub API; skipping prebuilt download."
		VERSION=""
	fi
fi

if [ -n "$VERSION" ]; then
	ASSET="nvs-${OS}-${ARCH}.tar.gz"
	URL="${REPO_URL}/releases/download/${VERSION}/${ASSET}"
	log "Trying prebuilt release: $URL"
	if [ "$DRYRUN" -eq 1 ]; then
		log "dry-run: download $URL"
	else
		if download "$URL" "$TMPDIR_WORK/$ASSET"; then
			if tar -xzf "$TMPDIR_WORK/$ASSET" -C "$TMPDIR_WORK"; then
				INSTALLED="$TMPDIR_WORK/nvs$EXE"
				log "Prebuilt release downloaded."
			else
				err "downloaded asset is not a valid tarball; falling back to source build."
			fi
		else
			log "No prebuilt ${OS}/${ARCH} asset for ${VERSION} (or no network); falling back to source build."
		fi
	fi
fi

# ---------------------------------------------------------------------------
# 3. Fall back: build from source with Go.
# ---------------------------------------------------------------------------
if [ -z "$INSTALLED" ]; then
	have_cmd go || {
		err "Go toolchain not found and no prebuilt release is available."
		err "Install Go (>= 1.21) from https://go.dev/dl/ and re-run, or"
		err "download a release manually: ${REPO_URL}/releases"
		exit 1
	}
	log "Go found: $(go version)"

	SRC_DIR=""
	if [ -f "./go.mod" ] && grep -q "navescript/nvs" ./go.mod 2>/dev/null; then
		# Running from a repo checkout: build in place.
		SRC_DIR="$PWD"
		log "Building from the local checkout: $SRC_DIR"
	else
		have_cmd git || {
			err "git not found; cannot fetch the NvS sources."
			err "Clone manually: git clone $REPO_URL && cd $REPO_NAME && sh install.sh"
			exit 1
		}
		SRC_DIR="$TMPDIR_WORK/src"
		BRANCH="master"
		if [ -n "$VERSION" ] && [ "$VERSION" != "latest" ]; then
			BRANCH="$VERSION"
		fi
		log "Cloning ${REPO_URL} (branch/tag: ${BRANCH}) ..."
		run git clone --depth 1 --branch "$BRANCH" "$REPO_URL" "$SRC_DIR" ||
			{
				err "git clone failed."
				exit 1
			}
	fi

	log "Compiling nvs (this can take a minute) ..."
	if [ "$DRYRUN" -eq 1 ]; then
		log "dry-run: (cd $SRC_DIR && go build -o nvs$EXE ./cmd/nvs)"
	else
		(cd "$SRC_DIR" && go build -trimpath -o "$TMPDIR_WORK/nvs$EXE" ./cmd/nvs) || {
			err "go build failed."
			exit 1
		}
	fi
	INSTALLED="$TMPDIR_WORK/nvs$EXE"
fi

# ---------------------------------------------------------------------------
# 4. Install into $PREFIX/bin.
# ---------------------------------------------------------------------------
BIN_DIR="$PREFIX/bin"
run mkdir -p "$BIN_DIR"
run cp "$INSTALLED" "$BIN_DIR/nvs$EXE"
run chmod +x "$BIN_DIR/nvs$EXE"

if [ "$DRYRUN" -eq 1 ]; then
	log "dry-run: installed nvs to $BIN_DIR/nvs$EXE (not really)"
	exit 0
fi

log ""
log "Installed: $BIN_DIR/nvs$EXE"
"$BIN_DIR/nvs$EXE" version 2>/dev/null || true

case ":$PATH:" in
*":$BIN_DIR:"*) ;;
*)
	log ""
	log "NOTE: $BIN_DIR is not on your PATH."
	log "Add it with:  export PATH=\"$BIN_DIR:\$PATH\""
	log "(put that line in ~/.bashrc, ~/.zshrc, or equivalent)"
	;;
esac
log "Done. Try:  nvs eval 'print \"hello, nave\"'"
