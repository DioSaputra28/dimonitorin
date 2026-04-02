# DiMonitorin

DiMonitorin is a lightweight single-node server monitoring tool built with Go, HTMX, Templ, Tailwind, SQLite, and ECharts.

It is designed for small VPS fleets and solo operators who want:
- one binary per server
- simple local persistence
- modern UI without a frontend framework
- CLI-first setup and recovery
- reverse-proxy-friendly deployment
- a Beszel-style install experience for Linux servers

## Features

- Linux host monitoring for CPU, load, memory, disks, uptime, network, top processes, and selected `systemd` services
- HTMX-powered partial refresh dashboard
- SQLite-backed local metric history and lightweight events
- Admin authentication with secure sessions and login lockout protection
- CLI commands for `init`, `run`, `doctor`, `config set`, `backup export`, `service install`, and password reset
- Non-interactive bootstrap flags for automation-friendly setup
- Bundled local static assets, including `ECharts`, `HTMX`, and compiled Tailwind CSS
- Remote-installer-ready release layout with `latest.json`, `checksums.txt`, and a public installer script

## Quick Start

### Remote install

Once the installer is hosted at `get.dimonitorin.dev`, the intended production install flow is:

```bash
curl -fsSL https://get.dimonitorin.dev/install.sh -o /tmp/install-dimonitorin.sh && chmod +x /tmp/install-dimonitorin.sh && sudo /tmp/install-dimonitorin.sh
sudo /usr/local/bin/dimonitorin --app-dir /opt/dimonitorin init
sudo systemctl enable --now dimonitorin
```

Optional auto-update setup:

```bash
curl -fsSL https://get.dimonitorin.dev/install.sh -o /tmp/install-dimonitorin.sh && chmod +x /tmp/install-dimonitorin.sh && sudo /tmp/install-dimonitorin.sh --auto-update
```

### Manual tarball install

```bash
make package VERSION=v0.1.0
sudo ./scripts/install.sh ./dist/dimonitorin_v0.1.0_linux_amd64.tar.gz
sudo /usr/local/bin/dimonitorin --app-dir /opt/dimonitorin init
sudo systemctl enable --now dimonitorin
```

### Source build

```bash
make deps
make build
sudo ./scripts/install.sh ./dist/build/dimonitorin
sudo /usr/local/bin/dimonitorin --app-dir /opt/dimonitorin init
sudo systemctl enable --now dimonitorin
```

If `make` is unavailable:

```bash
/root/go/bin/templ generate ./internal/views
npx tailwindcss -i ./static/src/input.css -o ./static/css/app.css --minify
cp node_modules/htmx.org/dist/htmx.min.js static/js/htmx.min.js
cp node_modules/echarts/dist/echarts.min.js static/js/echarts.min.js
gofmt -w $(find . -type f -name '*.go' -not -path './node_modules/*')
go build -trimpath -ldflags "-s -w -X main.version=dev -X main.commit=none -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o dist/build/dimonitorin ./cmd/dimonitorin
```

## Automation-friendly bootstrap

`init` now supports non-interactive setup for scripts and installer-driven flows:

```bash
printf 'supersecret123\n' | dimonitorin --app-dir /opt/dimonitorin init \
  --host 127.0.0.1 \
  --port 8080 \
  --non-interactive \
  --skip-service-discovery \
  --services nginx,postgresql,redis \
  --admin-user admin \
  --admin-password-stdin
```

## Documentation

- Development guide: [docs/DEVELOPMENT.md](/root/project/monitor-vps/docs/DEVELOPMENT.md)
- Install guide: [docs/INSTALL.md](/root/project/monitor-vps/docs/INSTALL.md)
- Packaging guide: [docs/PACKAGING.md](/root/project/monitor-vps/docs/PACKAGING.md)

## Common Commands

```bash
make build
make test
make package VERSION=v0.1.0
./dist/build/dimonitorin version
./dist/build/dimonitorin --app-dir /opt/dimonitorin doctor
./dist/build/dimonitorin --app-dir /opt/dimonitorin backup export
```

## Release Output

Packaging now produces:
- `dist/install.sh`
- `dist/latest.json`
- `dist/checksums.txt`
- `dist/releases/<version>/...`
- convenience tarballs in `dist/*.tar.gz`

## Notes

- Production HTTPS is expected to be handled by a reverse proxy such as Caddy or Nginx.
- Monitoring support in v1 is focused on Linux hosts with `systemd`.
- The remote installer is implemented in-repo; publishing it to `get.dimonitorin.dev` and the release artifacts to `downloads.dimonitorin.dev` is the final hosting step.
