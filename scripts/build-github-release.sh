#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist/github-release"
BUILD_DIR="${DIST_DIR}/build"
VERSION="${1:-${GITHUB_REF_NAME:-}}"
TARGET="${2:-all}"
# CI owns verification. Packaging consumes its exact-revision frontend artifact;
# standalone builds prepare the frontend once but do not rerun the test suites.
FRONTEND_DIR="${DENOVA_RELEASE_FRONTEND_DIR:-${ROOT_DIR}/web/dist}"

if [[ -z "${VERSION}" ]]; then
  if git -C "${ROOT_DIR}" describe --tags --exact-match >/dev/null 2>&1; then
    VERSION="$(git -C "${ROOT_DIR}" describe --tags --exact-match)"
  else
    VERSION="dev"
  fi
fi

TARGETS=(
  "darwin-arm64:darwin:arm64:denova:denova-updater:tar.gz"
  "darwin-x64:darwin:amd64:denova:denova-updater:tar.gz"
  "linux-arm64:linux:arm64:denova:denova-updater:tar.gz"
  "linux-x64:linux:amd64:denova:denova-updater:tar.gz"
  "windows-x64:windows:amd64:denova.exe:denova-updater.exe:zip"
)

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Error: command not found: $1" >&2
    exit 1
  fi
}

run_pnpm() {
  if command -v pnpm >/dev/null 2>&1; then
    pnpm "$@"
    return
  fi
  npx pnpm "$@"
}

copy_if_exists() {
  local from="$1"
  local to="$2"
  if [[ -e "${from}" ]]; then
    cp -R "${from}" "${to}"
  fi
}

checksum_file() {
  local file="$1"
  local name
  name="$(basename "${file}")"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${file}" | awk -v name="${name}" '{print $1 "  " name}'
    return
  fi
  shasum -a 256 "${file}" | awk -v name="${name}" '{print $1 "  " name}'
}

release_version_without_prefix() {
  printf '%s' "${VERSION#v}"
}

extract_release_brief() {
  local release_tag="$1"
  awk -v release_heading="## [${release_tag}]" '
    index($0, release_heading) == 1 { in_release = 1; next }
    in_release && /^## \[/ { exit }
    in_release && $0 == "### Brief / 简要说明" { in_brief = 1; found = 1; next }
    in_brief && /^### / { exit }
    in_brief { print }
    END {
      if (!found) {
        print "CHANGELOG.md is missing a Brief section for " release_heading > "/dev/stderr"
        exit 1
      }
    }
  '
}

validate_release_metadata() {
  if [[ "${VERSION}" == "dev" ]]; then
    return
  fi
  local expected web_version release_tag
  expected="$(release_version_without_prefix)"
  release_tag="v${expected}"
  web_version="$(node -p "require('./web/package.json').version")"
  if [[ "${web_version}" != "${expected}" ]]; then
    echo "Error: release ${release_tag} does not match web version ${web_version}" >&2
    exit 1
  fi
  if ! grep -Fq "## [${release_tag}]" CHANGELOG.md; then
    echo "Error: CHANGELOG.md is missing ${release_tag}" >&2
    exit 1
  fi
  extract_release_brief "${release_tag}" < CHANGELOG.md >/dev/null
  if ! grep -Fq "<strong>${release_tag}</strong>" README.md || ! grep -Fq "<strong>${release_tag}</strong>" README.en.md; then
    echo "Error: both README files must identify ${release_tag} as the current version" >&2
    exit 1
  fi
}

write_release_notes() {
  if [[ "${VERSION}" == "dev" ]]; then
    cat > "${DIST_DIR}/RELEASE_NOTES.md" <<'EOF'
# Denova development build

Built from the current working tree for local validation. See the Unreleased section of CHANGELOG.md for changes. This archive has not been published as a release.
EOF
    return
  fi
  local release_tag
  release_tag="v$(release_version_without_prefix)"
  {
    echo "# Denova ${release_tag}"
    echo
    echo "## Release highlights / 发布内容"
    echo
    extract_release_brief "${release_tag}" < CHANGELOG.md
    cat <<'EOF'

## Verification / 验证

- The release workflow requires successful CI for the exact tagged commit, including both Go modules, frontend unit tests, translations, and browser journeys against the built distribution.
- Packaging reuses the frontend from that CI run; platform archives are compiled in parallel without repeating the test suites.
- Packaging: five platform archives are generated from the same source revision and listed in `checksums.txt`.

发布流程要求标签对应提交的 CI 成功，覆盖两个 Go module、前端单测、双语键检查、生产构建及构建产物上的浏览器流程；复用该次 CI 的前端产物，并行生成五个平台压缩包及 `checksums.txt`。

## Install / 安装

macOS / Linux one-command install / 一键安装：

```bash
curl -fsSL https://github.com/alfredxw/denova/releases/latest/download/install.sh | sh
```

Or download the archive for your platform, verify it against `checksums.txt`, extract it, and run Denova from the extracted `denova` directory.

也可以下载对应平台压缩包，使用 `checksums.txt` 校验后解压，并在解压后的 `denova` 目录运行：

```bash
./denova
```

Windows:

```powershell
denova.exe
```

Checksum example / 校验示例：

```bash
shasum -a 256 -c checksums.txt
```
EOF
  } > "${DIST_DIR}/RELEASE_NOTES.md"
}

require_command go
require_command node
require_command tar

if [[ ! "${VERSION}" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]]; then
  echo "Error: invalid release version: ${VERSION}" >&2
  exit 1
fi
if [[ "${TARGET}" != all ]]; then
  selected=()
  for target in "${TARGETS[@]}"; do
    if [[ "${target%%:*}" == "${TARGET}" ]]; then selected+=("${target}"); fi
  done
  if [[ ${#selected[@]} -ne 1 ]]; then
    echo "Error: unsupported release target: ${TARGET}" >&2
    exit 1
  fi
  TARGETS=("${selected[@]}")
fi

echo "==> Building GitHub Release assets version=${VERSION} target=${TARGET}"
cd "${ROOT_DIR}"
validate_release_metadata
if [[ -z "${DENOVA_RELEASE_FRONTEND_DIR:-}" ]]; then
  echo "==> Preparing frontend (run CI separately before publishing)"
  run_pnpm -C "${ROOT_DIR}/web" install --frozen-lockfile
  run_pnpm -C "${ROOT_DIR}/web" check:i18n
  run_pnpm -C "${ROOT_DIR}/web" build
fi
if [[ ! -s "${FRONTEND_DIR}/index.html" ]]; then
  echo "Error: missing built frontend: ${FRONTEND_DIR}/index.html" >&2
  exit 1
fi
mkdir -p "${DIST_DIR}" "${BUILD_DIR}"
cp "${ROOT_DIR}/scripts/install.sh" "${DIST_DIR}/install.sh"
chmod 0755 "${DIST_DIR}/install.sh"
# Match the normal distribution: serve external assets and retain the embedded
# fallback. Only generated directories below this checkout are replaced.
rm -rf "${ROOT_DIR}/internal/webfs/dist"
cp -R "${FRONTEND_DIR}" "${ROOT_DIR}/internal/webfs/dist"

echo "==> Compiling and packaging"
for target in "${TARGETS[@]}"; do
  IFS=":" read -r key goos goarch exe updater_exe archive_type <<<"${target}"
  package_name="denova-${VERSION}-${key}"
  package_dir="${BUILD_DIR}/${package_name}/denova"
  rm -rf "${BUILD_DIR}/${package_name}"
  mkdir -p "${package_dir}"

  echo "  -> ${key}"
  binary_version="${VERSION#v}"
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -tags embedweb -trimpath -ldflags "-s -w -X denova/internal/buildinfo.Version=${binary_version}" -o "${package_dir}/${exe}" ./cmd/denova
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -trimpath -ldflags "-s -w -X denova/internal/buildinfo.Version=${binary_version}" -o "${package_dir}/${updater_exe}" ./cmd/denova-updater

  go run ./scripts/ripgrep-assets \
    -target "${key}" \
    -destination "${package_dir}"

  if [[ "${goos}" != "windows" ]]; then
    chmod 0755 "${package_dir}/${exe}"
    chmod 0755 "${package_dir}/${updater_exe}"
  fi

  cp -R "${FRONTEND_DIR}" "${package_dir}/web"
  cp -R "${ROOT_DIR}/skills" "${package_dir}/skills"
  copy_if_exists "${ROOT_DIR}/config.toml" "${package_dir}/"
  copy_if_exists "${ROOT_DIR}/README.md" "${package_dir}/"
  copy_if_exists "${ROOT_DIR}/README.en.md" "${package_dir}/"
  copy_if_exists "${ROOT_DIR}/CHANGELOG.md" "${package_dir}/"
  copy_if_exists "${ROOT_DIR}/LICENSE" "${package_dir}/"

  if [[ "${archive_type}" == "zip" ]]; then
    rm -f "${DIST_DIR}/${package_name}.zip"
    (
      cd "${BUILD_DIR}/${package_name}"
      if command -v zip >/dev/null 2>&1; then
        zip -qr "${DIST_DIR}/${package_name}.zip" denova
      elif command -v python3 >/dev/null 2>&1; then
        python3 -m zipfile -c "${DIST_DIR}/${package_name}.zip" denova
      elif command -v python >/dev/null 2>&1; then
        python -m zipfile -c "${DIST_DIR}/${package_name}.zip" denova
      else
        echo "Error: zip or Python is required to create the Windows archive" >&2
        exit 1
      fi
    )
  else
    (
      cd "${BUILD_DIR}/${package_name}"
      tar -czf "${DIST_DIR}/${package_name}.tar.gz" denova
    )
  fi
done

echo "==> Writing checksums.txt"
: > "${DIST_DIR}/checksums.txt"
for file in "${DIST_DIR}"/denova-"${VERSION}"-*; do
  checksum_file "${file}" >> "${DIST_DIR}/checksums.txt"
done

write_release_notes

echo "==> GitHub Release assets ready: ${DIST_DIR}"
find "${DIST_DIR}" -maxdepth 1 -type f -print | sort
