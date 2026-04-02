# Install Guide

This guide covers the intended remote installer flow, manual tarball install, source builds, and post-install operations.

For local coding and manual developer workflows, see [docs/DEVELOPMENT.md](/root/project/monitor-vps/docs/DEVELOPMENT.md).

## Requirements

### Runtime

- Linux host with `systemd`
- writable app directory, default: `/opt/dimonitorin`
- reverse proxy for public HTTPS if exposing the UI externally

### Development

- Go `1.26+`
- Node.js `24+`
- npm `11+`
- `templ` generator available at `/root/go/bin/templ` or installed via:

```bash
go install github.com/a-h/templ/cmd/templ@latest
```

## Recommended Install: Remote Installer

When the publishing step is wired to your infrastructure, the official install command is:

```bash
curl -fsSL https://get.dimonitorin.dev/install.sh -o /tmp/install-dimonitorin.sh && chmod +x /tmp/install-dimonitorin.sh && sudo /tmp/install-dimonitorin.sh
```

What it does:
- validates Linux + `systemd`
- detects `amd64` or `arm64`
- downloads the correct release archive from `downloads.dimonitorin.dev`
- verifies SHA256 using `latest.json` or `checksums.txt`
- installs the binary to `/usr/local/bin/dimonitorin`
- creates `/opt/dimonitorin`
- writes `/etc/systemd/system/dimonitorin.service`
- stores a local copy of the installer at `/opt/dimonitorin/install-dimonitorin.sh`
- prints the next-step commands for `init` and `systemctl enable --now`

### Optional flags

```bash
sudo /tmp/install-dimonitorin.sh --version v0.1.0
sudo /tmp/install-dimonitorin.sh --auto-update
sudo /tmp/install-dimonitorin.sh --app-dir /srv/dimonitorin --bin-path /usr/local/bin/dimonitorin
sudo /tmp/install-dimonitorin.sh --dry-run
sudo /tmp/install-dimonitorin.sh -u
sudo /tmp/install-dimonitorin.sh -u --purge
```

### Complete production bootstrap

```bash
curl -fsSL https://get.dimonitorin.dev/install.sh -o /tmp/install-dimonitorin.sh && chmod +x /tmp/install-dimonitorin.sh && sudo /tmp/install-dimonitorin.sh
printf 'supersecret123\n' | sudo /usr/local/bin/dimonitorin --app-dir /opt/dimonitorin init \
  --host 127.0.0.1 \
  --port 8080 \
  --non-interactive \
  --services nginx,postgresql,redis \
  --admin-user admin \
  --admin-password-stdin
sudo systemctl enable --now dimonitorin
```

## Manual Install From a Release Archive

Build or download a release archive, then install it with the same installer script:

```bash
make package VERSION=v0.1.0
sudo ./scripts/install.sh ./dist/dimonitorin_v0.1.0_linux_amd64.tar.gz
sudo /usr/local/bin/dimonitorin --app-dir /opt/dimonitorin init
sudo systemctl enable --now dimonitorin
```

You can also target a specific package layout directly under `dist/releases/<version>/`:

```bash
sudo ./scripts/install.sh ./dist/releases/v0.1.0/dimonitorin_v0.1.0_linux_amd64.tar.gz
```

## Development Install From Source

### 1. Install repo dependencies

```bash
make deps
```

### 2. Build local assets and binary

```bash
make build
```

If `make` is not installed:

```bash
/root/go/bin/templ generate ./internal/views
npx tailwindcss -i ./static/src/input.css -o ./static/css/app.css --minify
cp node_modules/htmx.org/dist/htmx.min.js static/js/htmx.min.js
cp node_modules/echarts/dist/echarts.min.js static/js/echarts.min.js
gofmt -w $(find . -type f -name '*.go' -not -path './node_modules/*')
mkdir -p dist/build
go build -trimpath -ldflags "-s -w -X main.version=dev -X main.commit=none -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o dist/build/dimonitorin ./cmd/dimonitorin
```

### 3. Install the locally built binary

```bash
sudo ./scripts/install.sh ./dist/build/dimonitorin
```

### 4. Initialize interactively

```bash
sudo /usr/local/bin/dimonitorin --app-dir /opt/dimonitorin init
```

During interactive `init`, DiMonitorin will:
- create the app directory structure
- create `config.yaml`
- create the SQLite database
- detect common `systemd` services
- ask which services should be tracked
- create the admin account
- write reverse proxy example files

### 5. Start the service

```bash
sudo systemctl enable --now dimonitorin
```

The default bind address is:

```text
127.0.0.1:8080
```

## Non-Interactive Bootstrap

`init` now supports automation-friendly flags:
- `--host`
- `--port`
- `--non-interactive`
- `--skip-service-discovery`
- `--services <csv>`
- `--admin-user <name>`
- `--admin-password-stdin`

Example:

```bash
printf 'supersecret123\n' | dimonitorin --app-dir /opt/dimonitorin init \
  --host 0.0.0.0 \
  --port 18080 \
  --non-interactive \
  --skip-service-discovery \
  --services nginx,redis \
  --admin-user ops \
  --admin-password-stdin
```

## Recommended Production Layout

### Binary

```text
/usr/local/bin/dimonitorin
```

### App data

```text
/opt/dimonitorin/
  config.yaml
  dimonitorin.db
  logs/
  backups/
  reverse-proxy.caddy
  reverse-proxy.nginx
  install-dimonitorin.sh
```

### systemd units

```text
/etc/systemd/system/dimonitorin.service
/etc/systemd/system/dimonitorin-update.service
/etc/systemd/system/dimonitorin-update.timer
```

The update units are only created when `--auto-update` is used.

## Reverse Proxy

DiMonitorin is designed to sit behind a reverse proxy for public HTTPS.

`init` writes two example files into the app directory:
- `reverse-proxy.caddy`
- `reverse-proxy.nginx`

### Caddy example

```caddyfile
example.com {
    reverse_proxy 127.0.0.1:8080
}
```

### Nginx example

```nginx
server {
    listen 443 ssl;
    server_name example.com;

    location / {
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_pass http://127.0.0.1:8080;
    }
}
```

## Post-Install Commands

### Check the installed version

```bash
dimonitorin version
```

### Validate installation

```bash
dimonitorin --app-dir /opt/dimonitorin doctor
```

### Update config values

```bash
dimonitorin --app-dir /opt/dimonitorin config set server.host 127.0.0.1
dimonitorin --app-dir /opt/dimonitorin config set server.port 8081
dimonitorin --app-dir /opt/dimonitorin config set sampling.interval 30s
dimonitorin --app-dir /opt/dimonitorin config set monitor.tracked_services nginx,postgresql,redis
```

### Reset admin password

```bash
dimonitorin --app-dir /opt/dimonitorin reset-password
```

### Export a backup

```bash
dimonitorin --app-dir /opt/dimonitorin backup export
```

## Upgrade Flow

### Manual upgrade with a specific release

```bash
sudo ./scripts/install.sh --version v0.1.0
sudo systemctl restart dimonitorin
```

### Auto-update mode

Install with:

```bash
sudo /tmp/install-dimonitorin.sh --auto-update
```

That writes and enables:
- `dimonitorin-update.service`
- `dimonitorin-update.timer`

The timer runs daily with randomized delay. When the installer is triggered by the auto-update service and the binary changes, `dimonitorin.service` is restarted automatically.

## Troubleshooting

### Verify service status

```bash
sudo systemctl status dimonitorin
```

### Read service logs

```bash
sudo journalctl -u dimonitorin -f
```

### Dry-run the installer

```bash
./scripts/install.sh --dry-run --version latest
```

### Validate app configuration and collectors

```bash
dimonitorin --app-dir /opt/dimonitorin doctor
```
