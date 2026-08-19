#!/bin/sh

set -eu

REPOSITORY="alfredxw/denova"
RELEASES_URL="https://github.com/${REPOSITORY}/releases"
MANAGED_MARKER=".installed-by-denova-install-sh"
LAUNCHER_MARKER="# Managed by Denova install.sh"

TEMP_DIR=""
STAGE_DIR=""
BACKUP_ROOT=""
LAUNCHER_TEMP=""
NEW_INSTALL_ACTIVE=0
INSTALL_COMMITTED=0

log() {
  printf '%s\n' "$*"
}

fail() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

cleanup() {
  status=$?
  trap - 0 HUP INT TERM
  set +e
  if [ "${status}" -ne 0 ] && [ "${INSTALL_COMMITTED}" -eq 0 ]; then
    if [ "${NEW_INSTALL_ACTIVE}" -eq 1 ] && [ -f "${INSTALL_DIR:-}/${MANAGED_MARKER}" ]; then
      rm -rf "${INSTALL_DIR}"
    fi
    if [ -n "${BACKUP_ROOT}" ] && [ -d "${BACKUP_ROOT}/denova" ] && [ ! -e "${INSTALL_DIR:-}" ]; then
      mv "${BACKUP_ROOT}/denova" "${INSTALL_DIR}"
    fi
  fi
  if [ -n "${LAUNCHER_TEMP}" ] && [ -e "${LAUNCHER_TEMP}" ]; then
    rm -f "${LAUNCHER_TEMP}"
  fi
  if [ -n "${STAGE_DIR}" ] && [ -d "${STAGE_DIR}" ]; then
    rm -rf "${STAGE_DIR}"
  fi
  if [ -n "${TEMP_DIR}" ] && [ -d "${TEMP_DIR}" ]; then
    rm -rf "${TEMP_DIR}"
  fi
  if [ -n "${BACKUP_ROOT}" ] && [ -d "${BACKUP_ROOT}" ] && [ ! -e "${BACKUP_ROOT}/denova" ]; then
    rm -rf "${BACKUP_ROOT}"
  fi
  exit "${status}"
}

trap cleanup 0
trap 'exit 1' HUP INT TERM

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

absolute_dir() {
  while [ "$1" != "/" ] && [ "${1%/}" != "$1" ]; do
    set -- "${1%/}" "$2"
  done
  case "$1" in
    /*) printf '%s\n' "$1" ;;
    *) fail "$2 must be an absolute path: $1" ;;
  esac
}

choose_data_dir() {
  default_data_dir="${HOME}/.denova"
  if [ -f "${INSTALL_DIR}/${MANAGED_MARKER}" ]; then
    installed_data_dir="$(sed -n '2p' "${INSTALL_DIR}/${MANAGED_MARKER}")"
    case "${installed_data_dir}" in
      /*) default_data_dir="${installed_data_dir}" ;;
    esac
  fi

  selected_data_dir="${DENOVA_DIR:-}"
  if [ -z "${selected_data_dir}" ] && [ -t 1 ]; then
    printf 'Denova user data directory [%s]: ' "${default_data_dir}" > /dev/tty
    IFS= read -r selected_data_dir < /dev/tty || fail "could not read the user data directory"
  fi
  [ -n "${selected_data_dir}" ] || selected_data_dir="${default_data_dir}"

  case "${selected_data_dir}" in
    '~') selected_data_dir="${HOME}" ;;
    '~/'*) selected_data_dir="${HOME}/${selected_data_dir#\~/}" ;;
  esac
  DATA_DIR="$(absolute_dir "${selected_data_dir}" "DENOVA_DIR")"
  [ "${DATA_DIR}" != "/" ] || fail "DENOVA_DIR cannot be /"
}

resolve_platform() {
  case "$(uname -s)" in
    Darwin) platform_os="darwin" ;;
    Linux) platform_os="linux" ;;
    *) fail "unsupported operating system: $(uname -s)" ;;
  esac

  case "$(uname -m)" in
    arm64 | aarch64) platform_arch="arm64" ;;
    x86_64 | amd64) platform_arch="x64" ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac

  PLATFORM="${platform_os}-${platform_arch}"
}

resolve_version() {
  VERSION="${DENOVA_VERSION:-}"
  if [ -z "${VERSION}" ] || [ "${VERSION}" = "latest" ]; then
    log "Resolving the latest Denova release..."
    latest_url="$(curl -fsSL -o /dev/null -w '%{url_effective}' "${RELEASES_URL}/latest")" || fail "could not resolve the latest Denova release"
    latest_url="${latest_url%/}"
    VERSION="${latest_url##*/}"
  fi

  case "${VERSION}" in
    v*) ;;
    *) VERSION="v${VERSION}" ;;
  esac

  if ! printf '%s\n' "${VERSION}" | grep -Eq '^v[0-9A-Za-z][0-9A-Za-z._-]*$'; then
    fail "invalid Denova version: ${VERSION}"
  fi
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
    return
  fi
  fail "SHA-256 verification requires sha256sum or shasum"
}

shell_quote() {
  printf "'"
  printf '%s' "$1" | sed "s/'/'\\\\''/g"
  printf "'"
}

write_launcher() {
  executable_quoted="$(shell_quote "${INSTALL_DIR}/denova")"
  data_dir_quoted="$(shell_quote "${DATA_DIR}")"
  {
    printf '%s\n' '#!/bin/sh'
    printf '%s\n' "${LAUNCHER_MARKER}"
    printf '%s\n' 'set -eu'
    printf '%s\n' 'if [ -z "${DENOVA_DIR:-}" ]; then'
    printf '  DENOVA_DIR=%s\n' "${data_dir_quoted}"
    printf '%s\n' '  export DENOVA_DIR'
    printf '%s\n' 'fi'
    printf 'exec %s "$@"\n' "${executable_quoted}"
  } > "${LAUNCHER_TEMP}"
  chmod 0755 "${LAUNCHER_TEMP}"
}

restore_previous_install() {
  if [ -e "${INSTALL_DIR}" ]; then
    rm -rf "${INSTALL_DIR}"
  fi
  if [ -n "${BACKUP_ROOT}" ] && [ -d "${BACKUP_ROOT}/denova" ]; then
    mv "${BACKUP_ROOT}/denova" "${INSTALL_DIR}"
  fi
  NEW_INSTALL_ACTIVE=0
}

for command_name in curl tar uname mktemp awk grep sed cp mv rm mkdir chmod; do
  require_command "${command_name}"
done

[ -n "${HOME:-}" ] || fail "HOME is required"

INSTALL_DIR="$(absolute_dir "${DENOVA_INSTALL_DIR:-${HOME}/.local/lib/denova}" "DENOVA_INSTALL_DIR")"
BIN_DIR="$(absolute_dir "${DENOVA_BIN_DIR:-${HOME}/.local/bin}" "DENOVA_BIN_DIR")"

[ "${INSTALL_DIR}" != "/" ] || fail "DENOVA_INSTALL_DIR cannot be /"
[ "${BIN_DIR}" != "/" ] || fail "DENOVA_BIN_DIR cannot be /"
case "${BIN_DIR}/" in
  "${INSTALL_DIR}/"*) fail "DENOVA_BIN_DIR cannot be the application directory or one of its children" ;;
esac

LAUNCHER_PATH="${BIN_DIR}/denova"
if [ -L "${INSTALL_DIR}" ]; then
  fail "DENOVA_INSTALL_DIR cannot be a symbolic link: ${INSTALL_DIR}"
fi
if [ -e "${INSTALL_DIR}" ] && [ ! -f "${INSTALL_DIR}/${MANAGED_MARKER}" ]; then
  fail "${INSTALL_DIR} already exists and is not managed by this installer; choose another DENOVA_INSTALL_DIR"
fi
if { [ -e "${LAUNCHER_PATH}" ] || [ -L "${LAUNCHER_PATH}" ]; } && ! grep -Fq "${LAUNCHER_MARKER}" "${LAUNCHER_PATH}" 2>/dev/null; then
  fail "${LAUNCHER_PATH} already exists and is not managed by this installer; choose another DENOVA_BIN_DIR"
fi

choose_data_dir
case "${DATA_DIR}/" in
  "${INSTALL_DIR}/"*) fail "DENOVA_DIR cannot be the application directory or one of its children" ;;
esac

resolve_platform
resolve_version

ARCHIVE_NAME="denova-${VERSION}-${PLATFORM}.tar.gz"
DOWNLOAD_BASE="${RELEASES_URL}/download/${VERSION}"

TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/denova-install.XXXXXX")"
ARCHIVE_PATH="${TEMP_DIR}/${ARCHIVE_NAME}"
CHECKSUMS_PATH="${TEMP_DIR}/checksums.txt"
EXTRACT_DIR="${TEMP_DIR}/extract"
mkdir -p "${EXTRACT_DIR}"

log "Downloading Denova ${VERSION} for ${PLATFORM}..."
curl -fsSL --retry 3 --retry-delay 1 -o "${ARCHIVE_PATH}" "${DOWNLOAD_BASE}/${ARCHIVE_NAME}" || fail "could not download ${ARCHIVE_NAME}"
curl -fsSL --retry 3 --retry-delay 1 -o "${CHECKSUMS_PATH}" "${DOWNLOAD_BASE}/checksums.txt" || fail "could not download checksums.txt"

expected_checksum="$(awk -v name="${ARCHIVE_NAME}" '$2 == name { print $1; exit }' "${CHECKSUMS_PATH}")"
[ -n "${expected_checksum}" ] || fail "checksums.txt does not contain ${ARCHIVE_NAME}"
actual_checksum="$(sha256_file "${ARCHIVE_PATH}")"
[ "${actual_checksum}" = "${expected_checksum}" ] || fail "SHA-256 verification failed for ${ARCHIVE_NAME}"
log "Verified SHA-256 checksum."

if ! tar -tzf "${ARCHIVE_PATH}" | awk '
  $0 == "denova" || index($0, "denova/") == 1 {
    count = split($0, part, "/")
    for (part_index = 1; part_index <= count; part_index++) {
      if (part[part_index] == "..") exit 1
    }
    next
  }
  { exit 1 }
'; then
  fail "release archive contains an invalid path"
fi

tar -xzf "${ARCHIVE_PATH}" -C "${EXTRACT_DIR}"
PACKAGE_DIR="${EXTRACT_DIR}/denova"
for required_path in denova denova-updater web skills; do
  [ -e "${PACKAGE_DIR}/${required_path}" ] || fail "release archive is missing denova/${required_path}"
done
chmod 0755 "${PACKAGE_DIR}/denova" "${PACKAGE_DIR}/denova-updater"

installed_version="$("${PACKAGE_DIR}/denova" --version)" || fail "downloaded Denova binary cannot run on this system"
[ "${installed_version}" = "${VERSION#v}" ] || fail "downloaded binary reports version ${installed_version}, expected ${VERSION#v}"

INSTALL_PARENT="${INSTALL_DIR%/*}"
[ -n "${INSTALL_PARENT}" ] || INSTALL_PARENT="/"
mkdir -p "${INSTALL_PARENT}" "${BIN_DIR}"
STAGE_DIR="$(mktemp -d "${INSTALL_PARENT}/.denova-stage.XXXXXX")"
cp -R "${PACKAGE_DIR}/." "${STAGE_DIR}/"
printf '%s\n%s\n' "${REPOSITORY}" "${DATA_DIR}" > "${STAGE_DIR}/${MANAGED_MARKER}"

if [ -f "${INSTALL_DIR}/config.toml" ]; then
  cp "${INSTALL_DIR}/config.toml" "${STAGE_DIR}/config.toml"
fi

LAUNCHER_TEMP="$(mktemp "${BIN_DIR}/.denova-launcher.XXXXXX")"
write_launcher

if [ -d "${INSTALL_DIR}" ]; then
  BACKUP_ROOT="$(mktemp -d "${INSTALL_PARENT}/.denova-backup.XXXXXX")"
  mv "${INSTALL_DIR}" "${BACKUP_ROOT}/denova"
fi

if ! mv "${STAGE_DIR}" "${INSTALL_DIR}"; then
  restore_previous_install
  fail "could not move Denova into ${INSTALL_DIR}"
fi
STAGE_DIR=""
NEW_INSTALL_ACTIVE=1

if ! mv -f "${LAUNCHER_TEMP}" "${LAUNCHER_PATH}"; then
  restore_previous_install
  fail "could not install the denova command into ${BIN_DIR}"
fi
LAUNCHER_TEMP=""
INSTALL_COMMITTED=1

if [ -n "${BACKUP_ROOT}" ] && [ -d "${BACKUP_ROOT}" ]; then
  rm -rf "${BACKUP_ROOT}"
  BACKUP_ROOT=""
fi

log ""
log "Denova ${VERSION} installed successfully."
log "  Command:     ${LAUNCHER_PATH}"
log "  Application: ${INSTALL_DIR}"
log "  User data:   ${DATA_DIR}"

case ":${PATH:-}:" in
  *":${BIN_DIR}:"*) log "Run 'denova' to start." ;;
  *)
    log ""
    log "Add ${BIN_DIR} to PATH, then run 'denova':"
    log "  export PATH=\"${BIN_DIR}:\$PATH\""
    ;;
esac
