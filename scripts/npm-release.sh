#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PACKAGE_DIR="${ROOT_DIR}/npm"

PUBLISH=0
SKIP_BUILD=0
PACK_TGZ=0
ACCESS="${NPM_ACCESS:-public}"
TAG="${NPM_TAG:-latest}"
REGISTRY="${NPM_REGISTRY:-}"
OTP="${NPM_OTP:-}"

usage() {
  cat <<'USAGE'
用法: scripts/npm-release.sh [选项]

默认只构建并预览 npm 包内容，不会发布。

选项:
  --publish             真实发布到 npm registry
  --pack                额外生成本地 .tgz 包文件
  --skip-build          跳过构建，直接使用 npm/ 目录当前内容
  --tag <tag>           npm dist-tag，默认 latest
  --access <access>     scoped package 访问级别，默认 public
  --registry <url>      指定 npm registry
  --otp <code>          npm 二次验证验证码
  -h, --help            显示帮助

示例:
  scripts/npm-release.sh
  scripts/npm-release.sh --pack
  scripts/npm-release.sh --publish
  scripts/npm-release.sh --publish --tag beta
  scripts/npm-release.sh --publish --registry https://registry.npmjs.org/
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --publish)
      PUBLISH=1
      shift
      ;;
    --pack)
      PACK_TGZ=1
      shift
      ;;
    --skip-build)
      SKIP_BUILD=1
      shift
      ;;
    --tag)
      TAG="${2:-}"
      shift 2
      ;;
    --access)
      ACCESS="${2:-}"
      shift 2
      ;;
    --registry)
      REGISTRY="${2:-}"
      shift 2
      ;;
    --otp)
      OTP="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "未知参数: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "错误: 未找到命令 $1" >&2
    exit 1
  fi
}

require_value() {
  local name="$1"
  local value="$2"
  if [[ -z "${value}" ]]; then
    echo "错误: ${name} 不能为空" >&2
    exit 1
  fi
}

require_command node
require_command npm
require_value "--tag" "${TAG}"
require_value "--access" "${ACCESS}"

if [[ "${SKIP_BUILD}" -eq 0 ]]; then
  echo "==> 构建 npm 发布目录"
  node "${ROOT_DIR}/scripts/build-npm-package.mjs"
else
  echo "==> 跳过构建，使用现有 npm/ 目录"
fi

cd "${PACKAGE_DIR}"

PACKAGE_NAME="$(node -p "JSON.parse(require('fs').readFileSync('package.json', 'utf8')).name")"
PACKAGE_VERSION="$(node -p "JSON.parse(require('fs').readFileSync('package.json', 'utf8')).version")"

echo "==> 发布目标"
echo "  包名: ${PACKAGE_NAME}"
echo "  版本: ${PACKAGE_VERSION}"
echo "  tag: ${TAG}"
echo "  access: ${ACCESS}"
if [[ -n "${REGISTRY}" ]]; then
  echo "  registry: ${REGISTRY}"
fi

PACK_ARGS=(pack --dry-run --ignore-scripts)
PUBLISH_ARGS=(publish --ignore-scripts --access "${ACCESS}" --tag "${TAG}")
WHOAMI_ARGS=(whoami)
if [[ -n "${REGISTRY}" ]]; then
  PACK_ARGS+=(--registry "${REGISTRY}")
  PUBLISH_ARGS+=(--registry "${REGISTRY}")
  WHOAMI_ARGS+=(--registry "${REGISTRY}")
fi
if [[ -n "${OTP}" ]]; then
  PUBLISH_ARGS+=(--otp "${OTP}")
fi

echo "==> 预览 npm 包内容"
npm "${PACK_ARGS[@]}"

if [[ "${PACK_TGZ}" -eq 1 ]]; then
  echo "==> 生成本地 tgz 包"
  TGZ_ARGS=(pack --ignore-scripts)
  if [[ -n "${REGISTRY}" ]]; then
    TGZ_ARGS+=(--registry "${REGISTRY}")
  fi
  npm "${TGZ_ARGS[@]}"
fi

if [[ "${PUBLISH}" -eq 0 ]]; then
  echo "==> dry run 完成，未发布"
  echo "    真实发布请执行: scripts/npm-release.sh --publish"
  exit 0
fi

echo "==> 检查 npm 登录状态"
if ! npm "${WHOAMI_ARGS[@]}" >/dev/null 2>&1; then
  echo "错误: 当前未登录 npm，请先执行 npm login" >&2
  exit 1
fi

echo "==> 发布到 npm registry"
npm "${PUBLISH_ARGS[@]}"
echo "==> 发布完成: ${PACKAGE_NAME}@${PACKAGE_VERSION}"
