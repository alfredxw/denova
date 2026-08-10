#!/bin/bash

# MiniMax 图像生成测试脚本
# 用于验证 Denova 的 MiniMax 原生适配器配置

set -e

echo "🧪 MiniMax 图像生成配置测试"
echo "================================"
echo ""

# 检查 Go 环境
echo "📋 检查开发环境..."
if ! command -v go &> /dev/null; then
    echo "❌ 错误：未找到 Go 编译器"
    echo "请先安装 Go 1.26.5 或更高版本"
    exit 1
fi
echo "✅ Go 版本: $(go version)"
echo ""

# 检查项目文件
echo "📂 检查项目文件..."
if [ ! -f "internal/imagegen/minimax_adapter.go" ]; then
    echo "❌ 错误：MiniMax 适配器文件不存在"
    exit 1
fi
echo "✅ MiniMax 适配器文件存在"
echo ""

# 运行测试
echo "🧪 运行单元测试..."
cd internal/imagegen
go test -v -run TestMiniMax

echo ""
echo "🎉 所有测试通过！"
echo ""
echo "📝 配置步骤："
echo "1. 获取 MiniMax API Key: https://platform.minimaxi.com"
echo "2. 在 config.toml 中添加 MiniMax 配置"
echo "3. 启动 Denova: ./scripts/bootstrap.sh"
echo "4. 在设置页测试图像生成功能"
echo ""
echo "📚 详细配置指南: docs/minimax-image-setup.md"