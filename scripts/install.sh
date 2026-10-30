#!/bin/sh
# Shipcheck one-line installer script
# Usage: curl -sSL https://raw.githubusercontent.com/deployxa/shipcheck/main/scripts/install.sh | sh
set -e

REPO="deployxa/shipcheck"
BIN_NAME="shipcheck"

# 1. Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux*)  OS="linux" ;;
  darwin*) OS="darwin" ;;
  mingw*|msys*|cygwin*) OS="windows" ;;
  *)
    echo "Error: Unsupported operating system $OS" >&2
    exit 1
    ;;
esac

# 2. Detect Architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *)
    echo "Error: Unsupported CPU architecture $ARCH" >&2
    exit 1
    ;;
esac

# 3. Determine Version
VERSION="${SHIPCHECK_VERSION:-}"
if [ -z "$VERSION" ]; then
  LATEST_URL="https://api.github.com/repos/${REPO}/releases/latest"
  TAG=$(curl -sSL "$LATEST_URL" 2>/dev/null | grep '"tag_name":' | head -n1 | cut -d '"' -f 4 || true)
  if [ -n "$TAG" ]; then
    VERSION="${TAG#v}"
  else
    VERSION="0.1.0-rc1"
  fi
fi

echo "Installing Shipcheck v${VERSION} for ${OS}/${ARCH}..."

# 4. Target archive
EXT="tar.gz"
if [ "$OS" = "windows" ]; then
  EXT="zip"
fi
ARCHIVE="shipcheck-${VERSION}-${OS}-${ARCH}.${EXT}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/v${VERSION}/${ARCHIVE}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/v${VERSION}/shipcheck-${VERSION}-checksums.sha256"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

echo "Downloading $DOWNLOAD_URL ..."
curl -sSL -o "$TMP_DIR/$ARCHIVE" "$DOWNLOAD_URL"

# 5. Checksum verification
if curl -sSL -f -o "$TMP_DIR/checksums.txt" "$CHECKSUM_URL" 2>/dev/null; then
  echo "Verifying SHA256 checksum..."
  (
    cd "$TMP_DIR"
    EXPECTED_SHA=$(grep "$ARCHIVE" checksums.txt | awk '{print $1}')
    if [ -n "$EXPECTED_SHA" ]; then
      if command -v sha256sum >/dev/null 2>&1; then
        ACTUAL_SHA=$(sha256sum "$ARCHIVE" | awk '{print $1}')
      elif command -v shasum >/dev/null 2>&1; then
        ACTUAL_SHA=$(shasum -a 256 "$ARCHIVE" | awk '{print $1}')
      fi
      if [ -n "$ACTUAL_SHA" ] && [ "$ACTUAL_SHA" != "$EXPECTED_SHA" ]; then
        echo "Error: Checksum verification failed!" >&2
        echo "Expected: $EXPECTED_SHA" >&2
        echo "Actual:   $ACTUAL_SHA" >&2
        exit 1
      fi
      echo "Checksum verified: $EXPECTED_SHA"
    fi
  )
fi

# 6. Extract
(
  cd "$TMP_DIR"
  if [ "$EXT" = "zip" ]; then
    unzip -q "$ARCHIVE"
  else
    tar -xzf "$ARCHIVE"
  fi
)

# 7. Install location
INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
  INSTALL_DIR="$HOME/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

mv "$TMP_DIR/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
chmod +x "$INSTALL_DIR/$BIN_NAME"

echo "Shipcheck installed successfully to $INSTALL_DIR/$BIN_NAME"

if ! command -v "$BIN_NAME" >/dev/null 2>&1; then
  echo "Note: $INSTALL_DIR is not in your PATH. Add it to your shell config:"
  echo "  export PATH=\"\$PATH:$INSTALL_DIR\""
fi
