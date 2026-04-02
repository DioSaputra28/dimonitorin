# Development Guide

This guide is for local development and manual testing on the same machine where you edit the code.

## Goals

Use this guide when you want to:
- install development dependencies
- build the project locally
- run DiMonitorin manually without `systemd`
- test CLI commands one by one
- simulate common real-world usage and recovery cases

## Development Requirements

Minimum tools:
- Go `1.26+`
- Node.js `24+`
- npm `11+`
- `templ`
- `gofmt`
- `find`

Recommended but optional:
- `make`
- `ripgrep` (`rg`)
- `curl`
- `jq`

Install `templ` if needed:

```bash
go install github.com/a-h/templ/cmd/templ@latest
```

## Project Setup

Move into the repo:

```bash
cd /root/project/monitor-vps
```

Install project dependencies:

```bash
npm install
go mod tidy
```

## Build For Development

### Option 1: Build with `make`

```bash
make build
```

### Option 2: Build without `make`

Use this if `make` is not installed on the machine:

```bash
/root/go/bin/templ generate ./internal/views
npx tailwindcss -i ./static/src/input.css -o ./static/css/app.css --minify
cp node_modules/htmx.org/dist/htmx.min.js static/js/htmx.min.js
cp node_modules/echarts/dist/echarts.min.js static/js/echarts.min.js
gofmt -w $(find . -type f -name '*.go' -not -path './node_modules/*')
mkdir -p dist/build
go build -trimpath -ldflags "-s -w -X main.version=dev -X main.commit=none -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o dist/build/dimonitorin ./cmd/dimonitorin
```

Resulting binary:

```text
dist/build/dimonitorin
```

Check the binary:

```bash
./dist/build/dimonitorin version
```

## Daily Development Loop

This is the most useful loop while building features.

### 1. Rebuild after code changes

```bash
make build
```

Or use the manual fallback build command from above.

### 2. Create a fresh local test app directory

```bash
rm -rf /tmp/dimonitorin-dev
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev init
```

### 3. Run the app manually

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev run
```

### 4. Open the UI

```text
http://127.0.0.1:8080/login
```

### 5. Stop the app

Press `Ctrl+C` in the terminal running `run`.

## Command Reference

Supported commands:
- `init`
- `run`
- `create-admin`
- `reset-password`
- `config set`
- `doctor`
- `backup export`
- `service install`
- `version`

### `version`

Show embedded build metadata.

```bash
./dist/build/dimonitorin version
```

### `init`

Create a new app directory, database, admin account, and reverse proxy examples.

Interactive:

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev init
```

Non-interactive:

```bash
printf 'supersecret123\n' | ./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev init \
  --host 127.0.0.1 \
  --port 8080 \
  --non-interactive \
  --skip-service-discovery \
  --services nginx,postgresql,redis \
  --admin-user admin \
  --admin-password-stdin
```

Notes:
- admin password minimum is `8` characters
- if config already exists, `init` will stop instead of overwriting it

### `run`

Start the web application and background collectors.

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev run
```

### `create-admin`

Create or update an admin account.

Interactive:

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev create-admin
```

Non-interactive:

```bash
printf 'supersecret123\n' | ./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev create-admin \
  --username admin \
  --password-stdin \
  --non-interactive
```

### `reset-password`

Reset an existing admin password.

Interactive:

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev reset-password
```

Non-interactive:

```bash
printf 'newpassword123\n' | ./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev reset-password \
  --username admin \
  --password-stdin \
  --non-interactive
```

### `config set`

Update a config value from the CLI.

Examples:

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev config set server.host 0.0.0.0
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev config set server.port 18080
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev config set sampling.interval 15s
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev config set monitor.tracked_services nginx,redis,postgresql
```

### `doctor`

Run health checks for config, database, admin bootstrap, and collector state.

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev doctor
```

### `backup export`

Create a `.tar.gz` backup containing config and SQLite DB.

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev backup export
```

### `service install`

Write a `systemd` service file.

Development-safe example that writes to a temp file instead of `/etc/systemd/system`:

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev service install \
  --bin-path /root/project/monitor-vps/dist/build/dimonitorin \
  --output /tmp/dimonitorin-dev/dimonitorin.service
```

Production example:

```bash
sudo /usr/local/bin/dimonitorin --app-dir /opt/dimonitorin service install
```

## Common Development Scenarios

### Scenario 1: First local run

Goal: get the dashboard running on the same machine.

Commands:

```bash
cd /root/project/monitor-vps
make build
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev init
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev run
```

Open:

```text
http://127.0.0.1:8080/login
```

### Scenario 2: Run on port `18080`

Goal: avoid port collision with another local app.

Commands:

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev config set server.port 18080
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev run
```

Open:

```text
http://127.0.0.1:18080/login
```

### Scenario 3: Access from another device on the same network

Goal: open DiMonitorin from your laptop or phone using the server IP.

Commands:

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev config set server.host 0.0.0.0
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev config set server.port 18080
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev run
```

Then open:

```text
http://SERVER-IP:18080/login
```

If it still fails, check:
- machine firewall
- VPS provider firewall/security group
- whether the app is really listening on `0.0.0.0:18080`

### Scenario 4: Admin password was never set or needs reset

Goal: recover access without recreating the app directory.

Commands:

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev reset-password
```

Or non-interactive:

```bash
printf 'newpassword123\n' | ./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev reset-password \
  --username admin \
  --password-stdin \
  --non-interactive
```

### Scenario 5: Confirm config and collectors are healthy

Goal: quickly verify whether the app directory is usable.

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev doctor
```

This is useful after:
- moving the app directory
- editing `config.yaml`
- restoring from backup
- debugging login/setup issues

### Scenario 6: Export a backup before risky changes

Goal: save config + DB before refactors or upgrades.

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev backup export
ls -lah /tmp/dimonitorin-dev/backups
```

### Scenario 7: Rebuild and replace the installed binary

Goal: update the installed server after code changes.

```bash
make build
sudo ./scripts/install.sh ./dist/build/dimonitorin
sudo systemctl restart dimonitorin
```

### Scenario 8: Simulate the remote installer locally

Goal: test `latest.json` and the installer flow before real domains are live.

Terminal 1:

```bash
./scripts/package-release.sh v0.1.0-localtest
python3 -m http.server 38180 --directory dist
```

Terminal 2:

```bash
tmp_root=$(mktemp -d)
DIMONITORIN_SKIP_SYSTEMCTL=1 ./scripts/install.sh \
  --version latest \
  --downloads-url http://127.0.0.1:38180 \
  --app-dir "$tmp_root/app" \
  --bin-path "$tmp_root/bin/dimonitorin" \
  --systemd-dir "$tmp_root/systemd"
```

## Useful Files During Development

Local app directory usually contains:

```text
/tmp/dimonitorin-dev/
  config.yaml
  dimonitorin.db
  backups/
  logs/
  reverse-proxy.caddy
  reverse-proxy.nginx
```

Project output directories:

```text
dist/build/
dist/releases/
static/css/
static/js/
```

## Troubleshooting

### `make: not found`

Use the manual build commands from the `Build For Development` section.

### `rg: not found`

The `Makefile` now falls back to `find`, so `make build` should still work. If you want `rg` anyway:

```bash
sudo apt update
sudo apt install -y ripgrep
```

### `templ: command not found`

Install it:

```bash
go install github.com/a-h/templ/cmd/templ@latest
```

### Port already in use

Switch ports:

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev config set server.port 18080
```

### Login fails after setup

Check whether admin exists and reset password if needed:

```bash
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev doctor
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev reset-password
```

### Need a totally fresh local environment

```bash
rm -rf /tmp/dimonitorin-dev
./dist/build/dimonitorin --app-dir /tmp/dimonitorin-dev init
```
