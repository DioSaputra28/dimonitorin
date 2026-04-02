#!/usr/bin/env bash
set -euo pipefail

APP_NAME="dimonitorin"
DEFAULT_VERSION="latest"
DEFAULT_APP_DIR="/opt/dimonitorin"
DEFAULT_BIN_PATH="/usr/local/bin/${APP_NAME}"
DEFAULT_SYSTEMD_DIR="/etc/systemd/system"
DOWNLOADS_BASE_URL="${DIMONITORIN_DOWNLOADS_BASE_URL:-https://downloads.dimonitorin.dev}"
GET_BASE_URL="${DIMONITORIN_GET_BASE_URL:-https://get.dimonitorin.dev}"
SYSTEMD_DIR="${DIMONITORIN_SYSTEMD_DIR:-$DEFAULT_SYSTEMD_DIR}"
SKIP_SYSTEMCTL="${DIMONITORIN_SKIP_SYSTEMCTL:-0}"
SELF_PATH="${BASH_SOURCE[0]:-$0}"

VERSION="$DEFAULT_VERSION"
APP_DIR="$DEFAULT_APP_DIR"
BIN_PATH="$DEFAULT_BIN_PATH"
DRY_RUN=0
UNINSTALL=0
PURGE=0
AUTO_UPDATE=0
SOURCE_OVERRIDE=""
ARCH=""
OS=""
UPDATED_BINARY=0
PACKAGE_INDEX_UPDATED=0

usage() {
  cat <<USAGE
DiMonitorin installer

Usage:
  ./install.sh [options] [local-binary-or-archive]

Options:
  -u, --uninstall           Remove the installed binary, unit files, and local installer copy
      --purge               Remove app data in --app-dir during uninstall
      --version VERSION     Install a specific version or 'latest' (default: latest)
      --app-dir PATH        App directory (default: $DEFAULT_APP_DIR)
      --bin-path PATH       Binary install path (default: $DEFAULT_BIN_PATH)
      --auto-update         Install and enable the auto-update timer
      --dry-run             Print planned actions without changing the system
      --downloads-url URL   Override downloads base URL (default: $DOWNLOADS_BASE_URL)
      --get-url URL         Override installer base URL (default: $GET_BASE_URL)
      --systemd-dir PATH    Override systemd unit directory for testing
  -h, --help                Show this help

Examples:
  curl -fsSL https://get.dimonitorin.dev/install.sh -o /tmp/install-dimonitorin.sh && chmod +x /tmp/install-dimonitorin.sh && sudo /tmp/install-dimonitorin.sh
  sudo /tmp/install-dimonitorin.sh --version v0.1.0
  sudo /tmp/install-dimonitorin.sh --auto-update
  sudo /tmp/install-dimonitorin.sh -u
USAGE
}

log() {
  printf '==> %s\n' "$*" >&2
}

warn() {
  printf 'warning: %s\n' "$*" >&2
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

run_cmd() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf '+ %s\n' "$*"
    return 0
  fi
  "$@"
}

run_shell() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf '+ %s\n' "$*"
    return 0
  fi
  bash -lc "$*"
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -u|--uninstall)
        UNINSTALL=1
        shift
        ;;
      --purge)
        PURGE=1
        shift
        ;;
      --version)
        VERSION="${2:-}"
        [[ -n "$VERSION" ]] || die "--version requires a value"
        shift 2
        ;;
      --app-dir)
        APP_DIR="${2:-}"
        [[ -n "$APP_DIR" ]] || die "--app-dir requires a value"
        shift 2
        ;;
      --bin-path)
        BIN_PATH="${2:-}"
        [[ -n "$BIN_PATH" ]] || die "--bin-path requires a value"
        shift 2
        ;;
      --auto-update)
        AUTO_UPDATE=1
        shift
        ;;
      --dry-run)
        DRY_RUN=1
        shift
        ;;
      --downloads-url)
        DOWNLOADS_BASE_URL="${2:-}"
        [[ -n "$DOWNLOADS_BASE_URL" ]] || die "--downloads-url requires a value"
        shift 2
        ;;
      --get-url)
        GET_BASE_URL="${2:-}"
        [[ -n "$GET_BASE_URL" ]] || die "--get-url requires a value"
        shift 2
        ;;
      --systemd-dir)
        SYSTEMD_DIR="${2:-}"
        [[ -n "$SYSTEMD_DIR" ]] || die "--systemd-dir requires a value"
        shift 2
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      --)
        shift
        while [[ $# -gt 0 ]]; do
          [[ -z "$SOURCE_OVERRIDE" ]] || die "unexpected extra argument: $1"
          SOURCE_OVERRIDE="$1"
          shift
        done
        ;;
      -* )
        die "unknown flag: $1"
        ;;
      *)
        [[ -z "$SOURCE_OVERRIDE" ]] || die "unexpected extra argument: $1"
        SOURCE_OVERRIDE="$1"
        shift
        ;;
    esac
  done
}

require_root() {
  if [[ "$DRY_RUN" -eq 1 ]]; then
    return 0
  fi
  if [[ "$(id -u)" -eq 0 ]]; then
    return 0
  fi
  if ! command -v sudo >/dev/null 2>&1; then
    die "sudo is required to install ${APP_NAME}"
  fi
  log "Elevating with sudo"
  exec sudo env \
    DIMONITORIN_DOWNLOADS_BASE_URL="$DOWNLOADS_BASE_URL" \
    DIMONITORIN_GET_BASE_URL="$GET_BASE_URL" \
    DIMONITORIN_SYSTEMD_DIR="$SYSTEMD_DIR" \
    DIMONITORIN_SKIP_SYSTEMCTL="$SKIP_SYSTEMCTL" \
    bash "$SELF_PATH" "$@"
}

detect_platform() {
  OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$OS" in
    linux) ;;
    *) die "unsupported operating system: $OS (Linux + systemd only in v1)" ;;
  esac

  case "$(uname -m)" in
    x86_64|amd64)
      ARCH="amd64"
      ;;
    aarch64|arm64)
      ARCH="arm64"
      ;;
    *)
      die "unsupported architecture: $(uname -m)"
      ;;
  esac
}

package_manager() {
  if command -v apt-get >/dev/null 2>&1; then
    echo apt
  elif command -v dnf >/dev/null 2>&1; then
    echo dnf
  elif command -v yum >/dev/null 2>&1; then
    echo yum
  elif command -v pacman >/dev/null 2>&1; then
    echo pacman
  elif command -v zypper >/dev/null 2>&1; then
    echo zypper
  else
    echo none
  fi
}

install_packages() {
  local manager
  manager="$(package_manager)"
  [[ "$manager" != "none" ]] || die "missing required tools and no supported package manager found"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    log "Would install packages: $* via $manager"
    return 0
  fi
  case "$manager" in
    apt)
      if [[ "$PACKAGE_INDEX_UPDATED" -eq 0 ]]; then
        apt-get update
        PACKAGE_INDEX_UPDATED=1
      fi
      DEBIAN_FRONTEND=noninteractive apt-get install -y "$@"
      ;;
    dnf)
      dnf install -y "$@"
      ;;
    yum)
      yum install -y "$@"
      ;;
    pacman)
      pacman -Sy --noconfirm "$@"
      ;;
    zypper)
      zypper --non-interactive install "$@"
      ;;
  esac
}

ensure_tool() {
  local cmd="$1"
  shift
  if command -v "$cmd" >/dev/null 2>&1; then
    return 0
  fi
  log "Installing missing dependency: $cmd"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    return 0
  fi
  install_packages "$@"
  command -v "$cmd" >/dev/null 2>&1 || die "required command '$cmd' is unavailable"
}

ensure_runtime_dependencies() {
  detect_platform
  ensure_tool curl curl
  ensure_tool tar tar
  ensure_tool sha256sum coreutils
  if [[ "$SKIP_SYSTEMCTL" == "1" ]]; then
    warn "Skipping systemctl checks because DIMONITORIN_SKIP_SYSTEMCTL=1"
    return 0
  fi
  if command -v systemctl >/dev/null 2>&1; then
    return 0
  fi
  log "Installing missing dependency: systemctl"
  install_packages systemd
  command -v systemctl >/dev/null 2>&1 || die "systemctl is required; DiMonitorin remote install supports Linux with systemd only"
}

json_get() {
  local json="$1"
  local key="$2"
  printf '%s' "$json" | tr -d '\n' | sed -n "s/.*\"${key}\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p"
}

fetch_latest_manifest() {
  curl -fsSL "${DOWNLOADS_BASE_URL%/}/latest.json"
}

checksum_from_release() {
  local version="$1"
  local artifact="$2"
  curl -fsSL "${DOWNLOADS_BASE_URL%/}/releases/${version}/checksums.txt" | awk -v file="$artifact" '$2 == file {print $1}'
}

maybe_systemctl() {
  if [[ "$SKIP_SYSTEMCTL" == "1" ]]; then
    printf '+ systemctl %s (skipped)\n' "$*"
    return 0
  fi
  run_cmd systemctl "$@"
}

write_text_file() {
  local path="$1"
  local body="$2"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf '+ write %s\n' "$path"
    return 0
  fi
  mkdir -p "$(dirname "$path")"
  printf '%s' "$body" > "$path"
}

update_service_path() {
  printf '%s/%s-update.service' "$SYSTEMD_DIR" "$APP_NAME"
}

update_timer_path() {
  printf '%s/%s-update.timer' "$SYSTEMD_DIR" "$APP_NAME"
}

render_update_service() {
  cat <<UNIT
[Unit]
Description=DiMonitorin auto-update
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
Environment=DIMONITORIN_AUTO_UPDATE=1
ExecStart=${APP_DIR}/install-dimonitorin.sh --version latest --app-dir ${APP_DIR} --bin-path ${BIN_PATH}
UNIT
}

render_update_timer() {
  cat <<UNIT
[Unit]
Description=Run DiMonitorin auto-update daily

[Timer]
OnCalendar=daily
RandomizedDelaySec=1h
Persistent=true

[Install]
WantedBy=timers.target
UNIT
}

persist_local_installer() {
  local target="${APP_DIR}/install-dimonitorin.sh"
  if [[ ! -f "$SELF_PATH" ]]; then
    warn "installer source is not a regular file; skipping local installer copy"
    return 0
  fi
  run_cmd mkdir -p "$APP_DIR"
  run_cmd install -m 0755 "$SELF_PATH" "$target"
}

remove_file_if_exists() {
  local path="$1"
  if [[ -e "$path" || -L "$path" ]]; then
    run_cmd rm -f "$path"
  fi
}

extract_archive_binary() {
  local archive="$1"
  local workdir="$2"
  tar -xzf "$archive" -C "$workdir"
  find "$workdir" -type f -name "$APP_NAME" | head -n 1
}

stage_binary() {
  local tmpdir="$1"
  local staged_bin=""
  if [[ -n "$SOURCE_OVERRIDE" ]]; then
    if [[ "$DRY_RUN" -eq 1 ]]; then
      printf '%s' "${tmpdir}/${APP_NAME}"
      return 0
    fi
    if [[ -f "$SOURCE_OVERRIDE" ]]; then
      if [[ "$SOURCE_OVERRIDE" == *.tar.gz ]]; then
        staged_bin="$(extract_archive_binary "$SOURCE_OVERRIDE" "$tmpdir")"
      else
        staged_bin="$SOURCE_OVERRIDE"
      fi
    else
      local source_name
      source_name="$(basename "$SOURCE_OVERRIDE")"
      local downloaded_path="${tmpdir}/${source_name}"
      log "Downloading override artifact from ${SOURCE_OVERRIDE}"
      run_cmd curl -fsSL "$SOURCE_OVERRIDE" -o "$downloaded_path"
      if [[ "$downloaded_path" == *.tar.gz ]]; then
        staged_bin="$(extract_archive_binary "$downloaded_path" "$tmpdir")"
      else
        staged_bin="$downloaded_path"
      fi
    fi
    [[ -n "$staged_bin" && -f "$staged_bin" ]] || die "failed to stage binary from override source"
    printf '%s' "$staged_bin"
    return 0
  fi

  local artifact_file expected_sha artifact_url manifest version_from_manifest
  if [[ "$VERSION" == "latest" ]]; then
    manifest="$(fetch_latest_manifest)"
    version_from_manifest="$(json_get "$manifest" version)"
    artifact_url="$(json_get "$manifest" "linux_${ARCH}_url")"
    expected_sha="$(json_get "$manifest" "linux_${ARCH}_sha256")"
    [[ -n "$version_from_manifest" ]] || die "latest.json did not include a version"
    [[ -n "$artifact_url" ]] || die "latest.json did not include a linux_${ARCH}_url"
    [[ -n "$expected_sha" ]] || die "latest.json did not include a linux_${ARCH}_sha256"
    VERSION="$version_from_manifest"
  else
    artifact_file="${APP_NAME}_${VERSION}_linux_${ARCH}.tar.gz"
    artifact_url="${DOWNLOADS_BASE_URL%/}/releases/${VERSION}/${artifact_file}"
    expected_sha="$(checksum_from_release "$VERSION" "$artifact_file")"
    [[ -n "$expected_sha" ]] || die "could not resolve checksum for ${artifact_file}"
  fi

  artifact_file="$(basename "$artifact_url")"
  local archive_path="${tmpdir}/${artifact_file}"
  log "Downloading ${artifact_url}"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf '%s' "${tmpdir}/${APP_NAME}"
    return 0
  fi
  run_cmd curl -fsSL "$artifact_url" -o "$archive_path"
  local actual_sha
  actual_sha="$(sha256sum "$archive_path" | awk '{print $1}')"
  [[ "$actual_sha" == "$expected_sha" ]] || die "checksum mismatch for ${artifact_file}"

  staged_bin="$(extract_archive_binary "$archive_path" "$tmpdir")"
  [[ -n "$staged_bin" && -f "$staged_bin" ]] || die "could not find ${APP_NAME} in downloaded archive"
  printf '%s' "$staged_bin"
}

install_binary() {
  local staged_bin="$1"
  local current_sha=""
  local next_sha="pending"
  if [[ -f "$BIN_PATH" ]]; then
    current_sha="$(sha256sum "$BIN_PATH" | awk '{print $1}')"
  fi
  if [[ "$DRY_RUN" -eq 0 ]]; then
    next_sha="$(sha256sum "$staged_bin" | awk '{print $1}')"
  fi

  run_cmd mkdir -p "$(dirname "$BIN_PATH")"
  if [[ -n "$current_sha" && "$DRY_RUN" -eq 0 && "$current_sha" == "$next_sha" ]]; then
    log "Binary already matches target artifact"
    UPDATED_BINARY=0
    return 0
  fi
  run_cmd install -m 0755 "$staged_bin" "$BIN_PATH"
  UPDATED_BINARY=1
}

install_main_service() {
  log "Writing ${APP_NAME} service unit"
  run_cmd "$BIN_PATH" --app-dir "$APP_DIR" service install --bin-path "$BIN_PATH" --output "${SYSTEMD_DIR}/${APP_NAME}.service"
  maybe_systemctl daemon-reload
}

install_auto_update() {
  local service_path timer_path
  service_path="$(update_service_path)"
  timer_path="$(update_timer_path)"
  write_text_file "$service_path" "$(render_update_service)"
  write_text_file "$timer_path" "$(render_update_timer)"
  maybe_systemctl daemon-reload
  maybe_systemctl enable --now "${APP_NAME}-update.timer"
}

restart_if_auto_update_changed_binary() {
  if [[ "$UPDATED_BINARY" -ne 1 || "${DIMONITORIN_AUTO_UPDATE:-0}" != "1" ]]; then
    return 0
  fi
  if [[ "$SKIP_SYSTEMCTL" == "1" ]]; then
    warn "Skipping auto-update restart because DIMONITORIN_SKIP_SYSTEMCTL=1"
    return 0
  fi
  if systemctl is-active --quiet "${APP_NAME}.service"; then
    log "Restarting ${APP_NAME}.service after binary update"
    systemctl restart "${APP_NAME}.service"
  fi
}

install_flow() {
  ensure_runtime_dependencies
  local tmpdir staged_bin
  tmpdir="$(mktemp -d)"
  trap "rm -rf '$tmpdir'" EXIT

  staged_bin="$(stage_binary "$tmpdir")"
  run_cmd mkdir -p "$APP_DIR"
  install_binary "$staged_bin"
  persist_local_installer
  install_main_service
  if [[ "$AUTO_UPDATE" -eq 1 ]]; then
    install_auto_update
  fi
  restart_if_auto_update_changed_binary

  log "Installed ${APP_NAME} ${VERSION}"
  printf '\nNext steps:\n'
  printf '  %s --app-dir %s init\n' "$BIN_PATH" "$APP_DIR"
  printf '  systemctl enable --now %s\n' "$APP_NAME"
  if [[ "$AUTO_UPDATE" -eq 1 ]]; then
    printf '  systemctl status %s-update.timer\n' "$APP_NAME"
  fi
}

uninstall_flow() {
  if [[ "$SKIP_SYSTEMCTL" != "1" && "$(command -v systemctl >/dev/null 2>&1; echo $?)" -eq 0 ]]; then
    maybe_systemctl disable --now "${APP_NAME}-update.timer" || true
    maybe_systemctl stop "${APP_NAME}.service" || true
  fi
  remove_file_if_exists "$(update_timer_path)"
  remove_file_if_exists "$(update_service_path)"
  remove_file_if_exists "${SYSTEMD_DIR}/${APP_NAME}.service"
  remove_file_if_exists "$BIN_PATH"
  remove_file_if_exists "${APP_DIR}/install-dimonitorin.sh"
  if [[ "$PURGE" -eq 1 ]]; then
    run_cmd rm -rf "$APP_DIR"
  fi
  maybe_systemctl daemon-reload || true
  log "Uninstall complete"
  if [[ "$PURGE" -eq 0 ]]; then
    printf 'App data preserved at %s\n' "$APP_DIR"
  fi
}

main() {
  parse_args "$@"
  require_root "$@"
  if [[ "$UNINSTALL" -eq 1 ]]; then
    uninstall_flow
  else
    install_flow
  fi
}

main "$@"
