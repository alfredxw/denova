#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd -P)"

process_cwd() {
    local pid="$1"

    if [ -e "/proc/${pid}/cwd" ]; then
        readlink "/proc/${pid}/cwd"
        return
    fi
    if command -v lsof >/dev/null 2>&1; then
        lsof -a -d cwd -p "${pid}" -Fn 2>/dev/null | sed -n 's/^n//p'
        return
    fi
    return 1
}

is_repository_backend() {
    local pid="$1"
    local cwd
    local process_name
    local command_line

    cwd="$(process_cwd "${pid}")" || return 1
    [ "${cwd}" = "${ROOT_DIR}" ] || return 1

    process_name="$(ps -p "${pid}" -o comm= 2>/dev/null)" || return 1
    process_name="${process_name#"${process_name%%[![:space:]]*}"}"
    process_name="${process_name%"${process_name##*[![:space:]]}"}"

    case "${process_name##*/}" in
      denova|denova.exe)
        return 0
        ;;
      go|go.exe)
        command_line="$(ps -p "${pid}" -o command= 2>/dev/null)" || return 1
        case "${command_line}" in
          *"go run ./cmd/denova"*)
            return 0
            ;;
        esac
        ;;
    esac

    return 1
}

if ! command -v pgrep >/dev/null 2>&1; then
    echo "错误 / Error: 未找到 pgrep，无法安全识别 Denova 后端进程。 / pgrep is required to identify the Denova backend safely." >&2
    exit 1
fi
if [ ! -e "/proc/$$/cwd" ] && ! command -v lsof >/dev/null 2>&1; then
    echo "错误 / Error: 无法读取进程工作目录；请先安装 lsof。 / Cannot inspect process working directories; install lsof first." >&2
    exit 1
fi

backend_pids=()
while IFS= read -r pid; do
    if [ -n "${pid}" ] && [ "${pid}" != "$$" ] && is_repository_backend "${pid}"; then
        backend_pids+=("${pid}")
    fi
done < <(
    {
        pgrep -x denova || true
        pgrep -x denova.exe || true
        pgrep -f 'go run ./cmd/denova([[:space:]]|$)' || true
    } | sort -u
)

if [ "${#backend_pids[@]}" -eq 0 ]; then
    echo "==> 未发现当前仓库中运行的 Denova 后端 / No running Denova backend found for this repository"
else
    echo "==> 正在停止 Denova 后端（PID: ${backend_pids[*]}） / Stopping Denova backend (PID: ${backend_pids[*]})"
    for pid in "${backend_pids[@]}"; do
        if kill -0 "${pid}" 2>/dev/null; then
            if ! kill -TERM "${pid}" 2>/dev/null && kill -0 "${pid}" 2>/dev/null; then
                echo "错误 / Error: 无法停止 Denova 后端进程 ${pid}。 / Failed to stop Denova backend process ${pid}." >&2
                exit 1
            fi
        fi
    done

    while :; do
        has_running_process=false
        for pid in "${backend_pids[@]}"; do
            if kill -0 "${pid}" 2>/dev/null; then
                has_running_process=true
                break
            fi
        done
        if [ "${has_running_process}" = false ]; then
            break
        fi
        sleep 0.1
    done
fi

echo "==> 正在重启 Denova 后端 / Restarting Denova backend"
exec "${SCRIPT_DIR}/bootstrap.sh" be
