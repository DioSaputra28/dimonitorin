# Packaging Guide

This document explains how DiMonitorin binaries, release archives, installer metadata, and publishable remote-install outputs are produced.

## Packaging Goals

The packaging flow is now built around a Beszel-style remote installer contract:
- `dist/install.sh` is the public installer script intended for `get.dimonitorin.dev`
- `dist/releases/<version>/` contains the versioned artifacts intended for `downloads.dimonitorin.dev`
- `dist/latest.json` points remote installers at the current stable Linux archives
- `dist/checksums.txt` and `dist/releases/<version>/checksums.txt` provide SHA256 verification
- manual tarball installs still work from the same outputs

## Version Metadata

The binary embeds three build fields:
- `version`
- `commit`
- `build date`

You can inspect them with:

```bash
dimonitorin version
```

## Local Build

Build a development binary with embedded metadata defaults:

```bash
make build
```

Direct equivalent without `make`:

```bash
/root/go/bin/templ generate ./internal/views
npx tailwindcss -i ./static/src/input.css -o ./static/css/app.css --minify
cp node_modules/htmx.org/dist/htmx.min.js static/js/htmx.min.js
cp node_modules/echarts/dist/echarts.min.js static/js/echarts.min.js
gofmt -w $(find . -type f -name '*.go' -not -path './node_modules/*')
mkdir -p dist/build
go build -trimpath -ldflags "-s -w -X main.version=dev -X main.commit=none -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o dist/build/dimonitorin ./cmd/dimonitorin
```

Output:

```text
dist/build/dimonitorin
```

## Release Package Build

Build release archives and metadata for all configured targets:

```bash
make package VERSION=v0.1.0
```

Direct equivalent without `make`:

```bash
./scripts/package-release.sh v0.1.0
```

Current targets:
- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`

Example output files:

```text
dist/install.sh
dist/latest.json
dist/checksums.txt
dist/releases/v0.1.0/dimonitorin_v0.1.0_linux_amd64.tar.gz
dist/releases/v0.1.0/dimonitorin_v0.1.0_linux_arm64.tar.gz
dist/releases/v0.1.0/dimonitorin_v0.1.0_darwin_amd64.tar.gz
dist/releases/v0.1.0/dimonitorin_v0.1.0_darwin_arm64.tar.gz
dist/releases/v0.1.0/checksums.txt
```

Convenience copies of the tarballs are also placed in:

```text
dist/*.tar.gz
```

## Release Archive Contents

Each archive contains:
- `dimonitorin`
- `README.md`
- `INSTALL.md`
- `PACKAGING.md`

## latest.json Contract

The remote installer resolves `latest` using `dist/latest.json`.

Example shape:

```json
{
  "version": "v0.1.0",
  "generated_at": "2026-04-02T00:00:00Z",
  "linux_amd64_url": "https://downloads.dimonitorin.dev/releases/v0.1.0/dimonitorin_v0.1.0_linux_amd64.tar.gz",
  "linux_amd64_sha256": "...",
  "linux_arm64_url": "https://downloads.dimonitorin.dev/releases/v0.1.0/dimonitorin_v0.1.0_linux_arm64.tar.gz",
  "linux_arm64_sha256": "..."
}
```

The installer also supports explicit versions by downloading:

```text
https://downloads.dimonitorin.dev/releases/<version>/checksums.txt
```

## Remote Installer Script

The public installer logic lives in:

[scripts/install.sh](/root/project/monitor-vps/scripts/install.sh)

It supports:
- remote installs from `latest.json`
- remote installs from explicit versions
- local binary or tarball installs for development/testing
- uninstall with optional `--purge`
- optional auto-update timer setup
- dry-run mode

Examples:

```bash
./scripts/install.sh --dry-run
./scripts/install.sh --version v0.1.0
./scripts/install.sh ./dist/dimonitorin_v0.1.0_linux_amd64.tar.gz
./scripts/install.sh ./dist/build/dimonitorin
./scripts/install.sh -u
```

## Checksums

Checksums are generated automatically by `scripts/package-release.sh`.

Primary checksum files:

```text
dist/checksums.txt
dist/releases/<version>/checksums.txt
```

## Publishing Layout

Intended hosting layout:

### Installer origin

```text
https://get.dimonitorin.dev/install.sh
```

Upload from:

```text
dist/install.sh
```

### Downloads origin

```text
https://downloads.dimonitorin.dev/latest.json
https://downloads.dimonitorin.dev/releases/<version>/...
```

Upload from:

```text
dist/latest.json
dist/releases/<version>/
```

## CI Packaging

A GitHub Actions workflow is included at:

[release.yml](/root/project/monitor-vps/.github/workflows/release.yml)

The workflow:
- installs Go and Node.js
- installs `templ`
- runs asset generation
- runs tests
- runs packaging
- uploads all generated files under `dist/`
- optionally publishes to S3-compatible origins when the required secrets are configured

Expected publish secrets:
- `DIMONITORIN_AWS_ACCESS_KEY_ID`
- `DIMONITORIN_AWS_SECRET_ACCESS_KEY`
- `DIMONITORIN_AWS_REGION`
- `DIMONITORIN_DOWNLOADS_BUCKET`
- `DIMONITORIN_GET_BUCKET`

## Future Packaging Options

Natural next steps after this release contract:
- signed checksums and signed release artifacts
- GitHub Releases publishing alongside custom CDN publish
- `.deb` packaging for Debian/Ubuntu
- `.rpm` packaging for RHEL/Alma/Rocky
