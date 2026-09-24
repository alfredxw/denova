#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEST_ROOT="$(mktemp -d "${TMPDIR:-/tmp}/denova-release-test.XXXXXX")"
cleanup() {
  # Never recursively remove a path outside the allocated fixture directory.
  case "${TEST_ROOT}" in
    "${TMPDIR:-/tmp}"/denova-release-test.*) rm -rf "${TEST_ROOT}" ;;
    *) echo "Error: unsafe fixture cleanup path" >&2; return 1 ;;
  esac
}
trap cleanup EXIT

mkdir -p "${TEST_ROOT}"/{scripts,web/dist,skills,internal/webfs,fake-bin}
cp "${ROOT_DIR}/scripts/build-github-release.sh" "${TEST_ROOT}/scripts/"
cp "${ROOT_DIR}/scripts/install.sh" "${TEST_ROOT}/scripts/"
printf '{"version":"1.2.3"}\n' > "${TEST_ROOT}/web/package.json"
printf 'tested frontend\n' > "${TEST_ROOT}/web/dist/index.html"
printf 'skill fixture\n' > "${TEST_ROOT}/skills/fixture.md"
printf '## [v1.2.3]\n### Brief / 简要说明\nRelease fixture.\n' > "${TEST_ROOT}/CHANGELOG.md"
printf '<strong>v1.2.3</strong>\n' > "${TEST_ROOT}/README.md"
cp "${TEST_ROOT}/README.md" "${TEST_ROOT}/README.en.md"

# Exercise real metadata, copying, tar/zip and checksum generation. Substitute
# only expensive compilation/downloads; invoking frontend tests is an error.
cat > "${TEST_ROOT}/fake-bin/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
command="$1"
shift
case "$command" in
  build)
    while [[ "$1" != -o ]]; do shift; done
    printf '#!/bin/sh\n# binary %s/%s\nexit 0\n' "$GOOS" "$GOARCH" > "$2"
    ;;
  run)
    while [[ "$1" != -target ]]; do shift; done
    target="$2"
    while [[ "$1" != -destination ]]; do shift; done
    mkdir -p "$2/tools" "$2/licenses/ripgrep"
    exe=rg
    if [[ "$target" == windows-x64 ]]; then exe=rg.exe; fi
    printf '#!/bin/sh\n# ripgrep fixture\nexit 0\n' > "$2/tools/$exe"
    chmod 755 "$2/tools/$exe"
    printf 'license fixture\n' > "$2/licenses/ripgrep/LICENSE-MIT"
    ;;
  *) echo "Unexpected Go command: $command" >&2; exit 1 ;;
esac
EOF
printf '#!/bin/sh\necho "Unexpected pnpm invocation" >&2\nexit 1\n' > "${TEST_ROOT}/fake-bin/pnpm"
chmod 755 "${TEST_ROOT}/fake-bin/"*
export PATH="${TEST_ROOT}/fake-bin:${PATH}"
export DENOVA_RELEASE_FRONTEND_DIR="${TEST_ROOT}/web/dist"

bash "${TEST_ROOT}/scripts/build-github-release.sh" v1.2.3 > "${TEST_ROOT}/build.log"
assets="${TEST_ROOT}/dist/github-release"
test "$(find "${assets}" -maxdepth 1 -name 'denova-*' -type f | wc -l | tr -d ' ')" = 5
test "$(wc -l < "${assets}/checksums.txt" | tr -d ' ')" = 5
if command -v sha256sum >/dev/null 2>&1; then
  (cd "${assets}" && sha256sum -c checksums.txt)
else
  (cd "${assets}" && shasum -a 256 -c checksums.txt)
fi
mkdir "${TEST_ROOT}/extracted"
tar -xzf "${assets}/denova-v1.2.3-linux-x64.tar.gz" -C "${TEST_ROOT}/extracted"
package="${TEST_ROOT}/extracted/denova"
test -x "${package}/denova"
test -x "${package}/denova-updater"
test -x "${package}/tools/rg"
test -s "${package}/licenses/ripgrep/LICENSE-MIT"
cmp "${package}/web/index.html" "${DENOVA_RELEASE_FRONTEND_DIR}/index.html"
cmp "${package}/skills/fixture.md" "${TEST_ROOT}/skills/fixture.md"
cmp "${TEST_ROOT}/internal/webfs/dist/index.html" "${DENOVA_RELEASE_FRONTEND_DIR}/index.html"

# Repackaging one target must keep other targets and discard stale ZIP entries.
rm "${TEST_ROOT}/skills/fixture.md"
bash "${TEST_ROOT}/scripts/build-github-release.sh" v1.2.3 windows-x64 > "${TEST_ROOT}/rebuild.log"
test -s "${assets}/denova-v1.2.3-linux-x64.tar.gz"
if command -v unzip >/dev/null 2>&1; then
  unzip -Z1 "${assets}/denova-v1.2.3-windows-x64.zip" > "${TEST_ROOT}/zip.txt"
else
  python -m zipfile -l "${assets}/denova-v1.2.3-windows-x64.zip" > "${TEST_ROOT}/zip.txt"
fi
grep -q 'denova/denova.exe' "${TEST_ROOT}/zip.txt"
grep -q 'denova/denova-updater.exe' "${TEST_ROOT}/zip.txt"
grep -q 'denova/tools/rg.exe' "${TEST_ROOT}/zip.txt"
if grep -q 'skills/fixture.md' "${TEST_ROOT}/zip.txt"; then exit 1; fi

if bash "${TEST_ROOT}/scripts/build-github-release.sh" v1.2.3 ../outside > /dev/null 2>&1; then exit 1; fi
if bash "${TEST_ROOT}/scripts/build-github-release.sh" ../outside > /dev/null 2>&1; then exit 1; fi
if bash "${TEST_ROOT}/scripts/build-github-release.sh" v9.9.9 > /dev/null 2>&1; then exit 1; fi
rm "${DENOVA_RELEASE_FRONTEND_DIR}/index.html"
if bash "${TEST_ROOT}/scripts/build-github-release.sh" v1.2.3 linux-x64 > /dev/null 2>&1; then exit 1; fi
test -s "${assets}/denova-v1.2.3-linux-x64.tar.gz"
printf 'Release packaging tests passed.\n'
