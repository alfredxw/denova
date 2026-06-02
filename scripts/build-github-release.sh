#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST_DIR="${ROOT_DIR}/dist/github-release"
BUILD_DIR="${DIST_DIR}/build"
VERSION="${1:-${GITHUB_REF_NAME:-}}"

if [[ -z "${VERSION}" ]]; then
  if git -C "${ROOT_DIR}" describe --tags --exact-match >/dev/null 2>&1; then
    VERSION="$(git -C "${ROOT_DIR}" describe --tags --exact-match)"
  else
    VERSION="dev"
  fi
fi

TARGETS=(
  "darwin-arm64:darwin:arm64:nova:tar.gz"
  "darwin-x64:darwin:amd64:nova:tar.gz"
  "linux-arm64:linux:arm64:nova:tar.gz"
  "linux-x64:linux:amd64:nova:tar.gz"
  "windows-x64:windows:amd64:nova.exe:zip"
)

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "错误: 未找到命令 $1" >&2
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

require_command go
require_command node
require_command tar

echo "==> 构建 GitHub Release 产物 version=${VERSION}"
cd "${ROOT_DIR}"
rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}" "${BUILD_DIR}"

echo "==> 构建前端"
run_pnpm -C "${ROOT_DIR}/web" install --frozen-lockfile
run_pnpm -C "${ROOT_DIR}/web" build

echo "==> 交叉编译并打包"
for target in "${TARGETS[@]}"; do
  IFS=":" read -r key goos goarch exe archive_type <<<"${target}"
  package_name="nova-${VERSION}-${key}"
  package_dir="${BUILD_DIR}/${package_name}/nova"
  mkdir -p "${package_dir}"

  echo "  -> ${key}"
  CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" \
    go build -trimpath -ldflags "-s -w" -o "${package_dir}/${exe}" ./cmd/nova

  if [[ "${goos}" != "windows" ]]; then
    chmod 0755 "${package_dir}/${exe}"
  fi

  cp -R "${ROOT_DIR}/web/dist" "${package_dir}/web"
  cp -R "${ROOT_DIR}/skills" "${package_dir}/skills"
  copy_if_exists "${ROOT_DIR}/config.toml" "${package_dir}/"
  copy_if_exists "${ROOT_DIR}/README.md" "${package_dir}/"
  copy_if_exists "${ROOT_DIR}/CHANGELOG.md" "${package_dir}/"
  copy_if_exists "${ROOT_DIR}/LICENSE" "${package_dir}/"

  if [[ "${archive_type}" == "zip" ]]; then
    require_command zip
    (
      cd "${BUILD_DIR}/${package_name}"
      zip -qr "${DIST_DIR}/${package_name}.zip" nova
    )
  else
    (
      cd "${BUILD_DIR}/${package_name}"
      tar -czf "${DIST_DIR}/${package_name}.tar.gz" nova
    )
  fi
done

echo "==> 生成 checksums.txt"
: > "${DIST_DIR}/checksums.txt"
for file in "${DIST_DIR}"/nova-*; do
  checksum_file "${file}" >> "${DIST_DIR}/checksums.txt"
done

cat > "${DIST_DIR}/RELEASE_NOTES.md" <<EOF
Nova ${VERSION}

下载对应平台压缩包，解压后进入 nova 目录运行：

\`\`\`bash
./nova
\`\`\`

Windows 用户运行：

\`\`\`powershell
nova.exe
\`\`\`

校验文件完整性请使用 checksums.txt。
EOF

echo "==> GitHub Release 产物已生成: ${DIST_DIR}"
find "${DIST_DIR}" -maxdepth 1 -type f -print | sort
