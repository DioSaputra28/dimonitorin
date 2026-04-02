#!/usr/bin/env bash
set -euo pipefail

APP_NAME="dimonitorin"
VERSION="${1:-dev}"
DIST_DIR="dist"
RELEASE_DIR="${DIST_DIR}/releases/${VERSION}"
TMP_DIR="${DIST_DIR}/tmp"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo none)"
DOWNLOADS_BASE_URL="${DIMONITORIN_DOWNLOADS_BASE_URL:-https://downloads.dimonitorin.dev}"
PLATFORMS=(
  "linux amd64"
  "linux arm64"
  "darwin amd64"
  "darwin arm64"
)

rm -rf "$TMP_DIR" "$RELEASE_DIR"
mkdir -p "$TMP_DIR" "$RELEASE_DIR" "$DIST_DIR"
rm -f "$DIST_DIR"/*.tar.gz "$DIST_DIR"/checksums.txt "$DIST_DIR"/latest.json "$DIST_DIR"/install.sh

for platform in "${PLATFORMS[@]}"; do
  read -r GOOS GOARCH <<<"$platform"
  BASENAME="${APP_NAME}_${VERSION}_${GOOS}_${GOARCH}"
  WORKDIR="${TMP_DIR}/${BASENAME}"
  ARCHIVE_PATH="${RELEASE_DIR}/${BASENAME}.tar.gz"
  mkdir -p "$WORKDIR"

  echo "Building $GOOS/$GOARCH"
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" \
    go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.buildDate=${BUILD_DATE}" \
    -o "${WORKDIR}/${APP_NAME}" ./cmd/dimonitorin

  cp README.md "${WORKDIR}/README.md"
  cp docs/INSTALL.md "${WORKDIR}/INSTALL.md"
  cp docs/PACKAGING.md "${WORKDIR}/PACKAGING.md"

  tar -C "$TMP_DIR" -czf "$ARCHIVE_PATH" "$BASENAME"
  cp "$ARCHIVE_PATH" "$DIST_DIR/${BASENAME}.tar.gz"
done

(
  cd "$RELEASE_DIR"
  sha256sum *.tar.gz > checksums.txt
)
cp "$RELEASE_DIR/checksums.txt" "$DIST_DIR/checksums.txt"
cp scripts/install.sh "$DIST_DIR/install.sh"
chmod +x "$DIST_DIR/install.sh"

linux_amd64_file="${APP_NAME}_${VERSION}_linux_amd64.tar.gz"
linux_arm64_file="${APP_NAME}_${VERSION}_linux_arm64.tar.gz"
linux_amd64_sha="$(awk -v file="$linux_amd64_file" '$2 == file {print $1}' "$RELEASE_DIR/checksums.txt")"
linux_arm64_sha="$(awk -v file="$linux_arm64_file" '$2 == file {print $1}' "$RELEASE_DIR/checksums.txt")"

cat > "$DIST_DIR/latest.json" <<JSON
{
  "version": "${VERSION}",
  "generated_at": "${BUILD_DATE}",
  "linux_amd64_url": "${DOWNLOADS_BASE_URL%/}/releases/${VERSION}/${linux_amd64_file}",
  "linux_amd64_sha256": "${linux_amd64_sha}",
  "linux_arm64_url": "${DOWNLOADS_BASE_URL%/}/releases/${VERSION}/${linux_arm64_file}",
  "linux_arm64_sha256": "${linux_arm64_sha}"
}
JSON

rm -rf "$TMP_DIR"
echo "Release artifacts are in $DIST_DIR/"
echo "Versioned release bundle: $RELEASE_DIR/"
