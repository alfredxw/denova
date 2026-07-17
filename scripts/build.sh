#!/bin/bash
set -e

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "${ROOT_DIR}"

OUTPUT_DIR="output"
VERSION="${DENOVA_VERSION:-${NOVA_VERSION:-$(node -p "require('./web/package.json').version" 2>/dev/null || echo dev)}}"

echo "==> 清理 output 目录"
rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

EMBED_TAG=""
echo "==> 构建前端"
if [ -d "web" ]; then
    cd web
    if [ ! -d "node_modules" ]; then
        echo "  安装依赖..."
        pnpm install
    fi
    pnpm build
    cd ..
    echo "  复制前端产物到 ${OUTPUT_DIR}/web/"
    cp -r web/dist "${OUTPUT_DIR}/web"
    echo "  准备内嵌前端资源（go:embed，构建标签 embedweb）"
    rm -rf internal/webfs/dist
    cp -r web/dist internal/webfs/dist
    EMBED_TAG="-tags embedweb"
else
    echo "警告: web/ 目录不存在，跳过前端构建（denova 将不含内嵌前端）"
fi

echo "==> 编译 denova"
go build ${EMBED_TAG} -ldflags "-X denova/internal/buildinfo.Version=${VERSION}" -o "${OUTPUT_DIR}/denova" ./cmd/denova/

echo "==> 编译 denova-updater"
go build -ldflags "-X denova/internal/buildinfo.Version=${VERSION}" -o "${OUTPUT_DIR}/denova-updater" ./cmd/denova-updater/

echo "==> 复制 skills 目录"
cp -r skills "${OUTPUT_DIR}/skills"

echo "==> 复制 config.toml"
if [ -f config.toml ]; then
    cp config.toml "${OUTPUT_DIR}/config.toml"
else
    echo "警告: config.toml 不存在，跳过复制"
fi

echo "==> 复制 CHANGELOG.md"
if [ -f CHANGELOG.md ]; then
    cp CHANGELOG.md "${OUTPUT_DIR}/CHANGELOG.md"
else
    echo "警告: CHANGELOG.md 不存在，跳过复制"
fi

echo "==> 构建完成"
echo "  输出目录: ${OUTPUT_DIR}/"
ls -la "${OUTPUT_DIR}/"
echo ""
echo "使用方式:"
echo "  cd ${OUTPUT_DIR} && ./denova --workspace /path/to/my-novel"
