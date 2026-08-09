#!/bin/sh

set -eu

ROOT_DIR="$(CDPATH= cd "$(dirname "$0")/.." && pwd -P)"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/denova-install-test.XXXXXX")"

cleanup() {
  rm -rf "${TEST_ROOT}"
}
trap cleanup 0
trap 'exit 1' HUP INT TERM

case "$(uname -s)" in
  Darwin) platform_os="darwin" ;;
  Linux) platform_os="linux" ;;
  *) printf 'Skipping installer test on unsupported OS.\n'; exit 0 ;;
esac
case "$(uname -m)" in
  arm64 | aarch64) platform_arch="arm64" ;;
  x86_64 | amd64) platform_arch="x64" ;;
  *) printf 'Skipping installer test on unsupported architecture.\n'; exit 0 ;;
esac

VERSION="v9.8.7"
PLATFORM="${platform_os}-${platform_arch}"
ARCHIVE_NAME="denova-${VERSION}-${PLATFORM}.tar.gz"
RELEASE_DIR="${TEST_ROOT}/release"
PACKAGE_DIR="${TEST_ROOT}/package/denova"
FAKE_BIN="${TEST_ROOT}/fake-bin"
HOME_DIR="${TEST_ROOT}/home"
mkdir -p "${RELEASE_DIR}" "${PACKAGE_DIR}/web" "${PACKAGE_DIR}/skills" "${PACKAGE_DIR}/tools" "${PACKAGE_DIR}/licenses" "${FAKE_BIN}" "${HOME_DIR}"

cat > "${PACKAGE_DIR}/denova" <<'EOF'
#!/bin/sh
if [ "${1:-}" = "--version" ]; then
  printf '9.8.7\n'
  exit 0
fi
printf '%s|%s\n' "${DENOVA_DIR:-}" "$*"
EOF
cat > "${PACKAGE_DIR}/denova-updater" <<'EOF'
#!/bin/sh
exit 0
EOF
chmod 0755 "${PACKAGE_DIR}/denova" "${PACKAGE_DIR}/denova-updater"
printf 'fixture\n' > "${PACKAGE_DIR}/web/index.html"
printf 'fixture\n' > "${PACKAGE_DIR}/skills/README.md"
printf 'fixture\n' > "${PACKAGE_DIR}/tools/rg"
printf 'fixture\n' > "${PACKAGE_DIR}/licenses/ripgrep-MIT.txt"
printf 'fixture\n' > "${PACKAGE_DIR}/config.toml"

(cd "${TEST_ROOT}/package" && tar -czf "${RELEASE_DIR}/${ARCHIVE_NAME}" denova)
if command -v sha256sum >/dev/null 2>&1; then
  checksum="$(sha256sum "${RELEASE_DIR}/${ARCHIVE_NAME}" | awk '{print $1}')"
else
  checksum="$(shasum -a 256 "${RELEASE_DIR}/${ARCHIVE_NAME}" | awk '{print $1}')"
fi
printf '%s  %s\n' "${checksum}" "${ARCHIVE_NAME}" > "${RELEASE_DIR}/checksums.txt"

cat > "${FAKE_BIN}/curl" <<'EOF'
#!/bin/sh
set -eu
output=""
write_format=""
url=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) output="$2"; shift 2 ;;
    -w) write_format="$2"; shift 2 ;;
    --retry | --retry-delay) shift 2 ;;
    -*) shift ;;
    *) url="$1"; shift ;;
  esac
done
case "${url}" in
  */releases/latest)
    [ -z "${write_format}" ] || printf '%s' 'https://github.com/alfredxw/denova/releases/tag/v9.8.7'
    ;;
  */checksums.txt) cp "${DENOVA_INSTALL_TEST_RELEASE_DIR}/checksums.txt" "${output}" ;;
  */denova-v9.8.7-*.tar.gz)
    name="${url##*/}"
    cp "${DENOVA_INSTALL_TEST_RELEASE_DIR}/${name}" "${output}"
    ;;
  *) printf 'Unexpected installer URL: %s\n' "${url}" >&2; exit 1 ;;
esac
EOF
chmod 0755 "${FAKE_BIN}/curl"

PATH="${FAKE_BIN}:${PATH}" \
HOME="${HOME_DIR}" \
DENOVA_INSTALL_TEST_RELEASE_DIR="${RELEASE_DIR}" \
sh "${ROOT_DIR}/scripts/install.sh" >/dev/null

LAUNCHER="${HOME_DIR}/.local/bin/denova"
INSTALL_DIR="${HOME_DIR}/.local/lib/denova"
[ -x "${LAUNCHER}" ]
[ -x "${INSTALL_DIR}/denova" ]
[ "$("${LAUNCHER}" --version)" = "9.8.7" ]
[ "$(HOME="${HOME_DIR}" XDG_DATA_HOME= "${LAUNCHER}" hello world)" = "${HOME_DIR}/.local/share/denova|hello world" ]
[ "$(DENOVA_DIR="${TEST_ROOT}/custom-data" "${LAUNCHER}" hello)" = "${TEST_ROOT}/custom-data|hello" ]

printf 'preserved = true\n' > "${INSTALL_DIR}/config.toml"
PATH="${FAKE_BIN}:${PATH}" \
HOME="${HOME_DIR}" \
DENOVA_INSTALL_TEST_RELEASE_DIR="${RELEASE_DIR}" \
DENOVA_VERSION="9.8.7" \
sh "${ROOT_DIR}/scripts/install.sh" >/dev/null
grep -Fq 'preserved = true' "${INSTALL_DIR}/config.toml"

printf 'Installer tests passed.\n'
