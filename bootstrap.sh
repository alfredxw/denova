#!/bin/bash
set -e

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
BACKEND_PORT="${NOVA_BACKEND_PORT:-8080}"
FRONTEND_PORT="${NOVA_FRONTEND_PORT:-5173}"
FRONTEND_URL="http://localhost:${FRONTEND_PORT}"
BACKEND_URL="http://localhost:${BACKEND_PORT}"
FRONTEND_BIND_HOST="${NOVA_FRONTEND_HOST:-}"

cd "${ROOT_DIR}"

MODE="all"  # all | fe | be
if [ $# -gt 0 ] && [[ "$1" != --* ]]; then
    MODE="$1"
    shift
fi

usage() {
    echo "用法: ./bootstrap.sh [all|fe|be] [options]"
    echo "  all  - 启动前后端 (默认)"
    echo "  fe   - 仅启动前端 (Vite dev server)"
    echo "  be   - 仅启动后端 (Go server)"
    echo ""
    echo "前端选项:"
    echo "  --lan          允许同一局域网设备访问前端，等同于 --host 0.0.0.0"
    echo "  --host <host>  指定 Vite dev server 监听地址"
}

detect_lan_address() {
    local addr

    if command -v ipconfig >/dev/null 2>&1; then
        for iface in en0 en1 en2 en3; do
            addr="$(ipconfig getifaddr "${iface}" 2>/dev/null || true)"
            if [ -n "${addr}" ]; then
                echo "${addr}"
                return
            fi
        done
    fi

    if command -v hostname >/dev/null 2>&1; then
        addr="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
        if [ -n "${addr}" ]; then
            echo "${addr}"
            return
        fi
    fi

    if command -v ifconfig >/dev/null 2>&1; then
        addr="$(ifconfig | awk '/inet / && $2 != "127.0.0.1" {print $2; exit}' || true)"
        if [ -n "${addr}" ]; then
            echo "${addr}"
            return
        fi
    fi
}

while [ $# -gt 0 ]; do
    case "$1" in
      --lan)
        FRONTEND_BIND_HOST="0.0.0.0"
        shift
        ;;
      --host)
        if [ $# -lt 2 ]; then
            echo "错误: --host 需要指定监听地址"
            exit 1
        fi
        FRONTEND_BIND_HOST="$2"
        shift 2
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "错误: 未知参数 $1"
        usage
        exit 1
        ;;
    esac
done

case "$MODE" in
  fe|frontend)
    echo "==> Nova 前端开发服务启动"
    echo "  前端地址: ${FRONTEND_URL}"
    if [ "${FRONTEND_BIND_HOST}" = "0.0.0.0" ]; then
        LAN_ADDRESS="$(detect_lan_address)"
        if [ -n "${LAN_ADDRESS}" ]; then
            echo "  局域网地址: http://${LAN_ADDRESS}:${FRONTEND_PORT}"
        else
            echo "  局域网地址: http://<本机局域网IP>:${FRONTEND_PORT}"
        fi
    elif [ -n "${FRONTEND_BIND_HOST}" ]; then
        echo "  监听地址: ${FRONTEND_BIND_HOST}"
    fi
    echo ""

    if ! command -v pnpm >/dev/null 2>&1; then
        echo "错误: 未找到 pnpm，请先安装 pnpm"
        exit 1
    fi

    if [ ! -d "web/node_modules" ]; then
        echo "==> 安装前端依赖"
        (cd web && pnpm install)
    fi

    echo "  按 Ctrl+C 停止服务"
    if [ -n "${FRONTEND_BIND_HOST}" ]; then
        cd web && exec pnpm dev --host "${FRONTEND_BIND_HOST}" --port "${FRONTEND_PORT}"
    fi
    cd web && exec pnpm dev --port "${FRONTEND_PORT}"
    ;;

  be|backend)
    echo "==> Nova 后端开发服务启动"
    echo "  后端地址: ${BACKEND_URL}"
    echo ""

    echo "==> 拉取 Go 依赖"
    go mod tidy

    echo "  按 Ctrl+C 停止服务"
    exec go run ./cmd/nova --port "${BACKEND_PORT}" --no-open
    ;;

  all)
    echo "==> Nova 开发服务启动"
    echo "  前端地址: ${FRONTEND_URL}"
    echo "  后端地址: ${BACKEND_URL}"
    echo ""

    if ! command -v pnpm >/dev/null 2>&1; then
        echo "错误: 未找到 pnpm，请先安装 pnpm"
        exit 1
    fi

    if [ ! -d "web/node_modules" ]; then
        echo "==> 安装前端依赖"
        (cd web && pnpm install)
    fi

    echo "==> 拉取 Go 依赖"
    go mod tidy

    echo "==> 启动前后端"
    echo "  按 Ctrl+C 停止服务"
    echo ""

    exec go run ./cmd/nova --port "${BACKEND_PORT}" --dev --no-open
    ;;

  *)
    usage
    exit 1
    ;;
esac
