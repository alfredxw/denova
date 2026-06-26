#!/bin/bash
set -e

OUTPUT_DIR="output"
VERSION="${NOVA_VERSION:-$(node -p "require('./web/package.json').version" 2>/dev/null || echo dev)}"

echo "==> 清理 output 目录"
rm -rf "${OUTPUT_DIR}"
mkdir -p "${OUTPUT_DIR}"

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
else
    echo "警告: web/ 目录不存在，跳过前端构建"
fi

echo "==> 编译 nova"
go build -ldflags "-X nova/internal/buildinfo.Version=${VERSION}" -o "${OUTPUT_DIR}/nova" ./cmd/nova/

echo "==> 编译 nova-updater"
go build -ldflags "-X nova/internal/buildinfo.Version=${VERSION}" -o "${OUTPUT_DIR}/nova-updater" ./cmd/nova-updater/

echo "==> 复制 skills 目录"
cp -r skills "${OUTPUT_DIR}/skills"

echo "==> 复制 config.toml"
if [ -f config.toml ]; then
    cp config.toml "${OUTPUT_DIR}/config.toml"
else
    echo "警告: config.toml 不存在，跳过复制"
fi

echo "==> 构建完成"
echo "  输出目录: ${OUTPUT_DIR}/"
ls -la "${OUTPUT_DIR}/"
echo ""
echo "使用方式:"
echo "  cd ${OUTPUT_DIR} && ./nova --workspace /path/to/my-novel"
