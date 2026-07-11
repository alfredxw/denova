#!/usr/bin/env pwsh
# bootstrap.ps1 - Denova 开发服务启动脚本 (Windows PowerShell 版本)

$ErrorActionPreference = "Stop"

# ---------- 辅助函数 ----------
function Read-ConfigValue {
    param(
        [string]$path,
        [string]$key
    )
    if (-not (Test-Path $path)) {
        return $null
    }
    $content = Get-Content $path -Raw
    # 匹配 key = value，忽略注释行（以#开头），去除引号和首尾空格
    if ($content -match "(?m)^\s*$key\s*=\s*(?<val>.*?)(\s*#.*)?$") {
        $val = $Matches['val'].Trim()
        # 去除首尾引号
        if ($val -match '^"(.*)"$') {
            $val = $Matches[1]
        } elseif ($val -match "^'(.*)'$") {
            $val = $Matches[1]
        }
        return $val
    }
    return $null
}

function Test-ValidPort {
    param([string]$port)
    if ($port -match '^\d+$') {
        $num = [int]$port
        return ($num -ge 1 -and $num -le 65535)
    }
    return $false
}

function Expand-Path {
    param([string]$path)
    if (-not $path) { return $path }
    if ($path -eq "~") {
        return $env:USERPROFILE
    }
    if ($path -like "~/*") {
        return Join-Path $env:USERPROFILE ($path.Substring(2))
    }
    return $path
}

function Get-DefaultDataDir {
    if ((Test-Path ".nova") -and (-not (Test-Path ".denova"))) {
        return "./.nova"
    }
    return "./.denova"
}

function Get-StartupDataDir {
    if ($env:DENOVA_DIR) {
        return (Expand-Path $env:DENOVA_DIR)
    }
    if ($env:NOVA_DIR) {
        return (Expand-Path $env:NOVA_DIR)
    }
    $configured = Read-ConfigValue "config.toml" "denova_dir"
    if (-not $configured) {
        $configured = Read-ConfigValue "config.toml" "nova_dir"
    }
    if ($configured) {
        return (Expand-Path $configured)
    }
    return (Get-DefaultDataDir)
}

function Resolve-Port {
    param(
        [string]$currentEnv,
        [string]$legacyEnv,
        [string]$key,
        [string]$fallback
    )
    $port = $fallback

    $value = Read-ConfigValue "config.toml" $key
    if (Test-ValidPort $value) { $port = $value }

    $dataDir = Get-StartupDataDir
    $value = Read-ConfigValue (Join-Path $dataDir "config.toml") $key
    if (Test-ValidPort $value) { $port = $value }

    if (Test-ValidPort $legacyEnv) { $port = $legacyEnv }
    if (Test-ValidPort $currentEnv) { $port = $currentEnv }

    return $port
}

function Detect-LanAddress {
    # 获取第一个非回环 IPv4 地址
    try {
        $addr = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction Stop |
                Where-Object { $_.InterfaceAlias -notlike "*Loopback*" -and $_.IPAddress -notlike "127.*" } |
                Select-Object -First 1 -ExpandProperty IPAddress
        if ($addr) { return $addr }
    } catch {
        # 降级使用 ipconfig 解析
        $output = ipconfig | Select-String "IPv4" | ForEach-Object { $_ -replace '.*: ', '' }
        foreach ($ip in $output) {
            if ($ip -and $ip -notlike "127.*") {
                return $ip.Trim()
            }
        }
    }
    return $null
}

# ---------- 主逻辑 ----------
$ROOT_DIR = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $ROOT_DIR

# 解析端口
$BACKEND_PORT = Resolve-Port $env:DENOVA_BACKEND_PORT $env:NOVA_BACKEND_PORT "backend_port" "8080"
$FRONTEND_PORT = Resolve-Port $env:DENOVA_FRONTEND_PORT $env:NOVA_FRONTEND_PORT "frontend_port" "5173"
$FRONTEND_URL = "http://localhost:$FRONTEND_PORT"
$BACKEND_URL = "http://localhost:$BACKEND_PORT"

# 解析模式
$MODE = "all"
$FRONTEND_BIND_HOST = $env:DENOVA_FRONTEND_HOST
if (-not $FRONTEND_BIND_HOST) { $FRONTEND_BIND_HOST = $env:NOVA_FRONTEND_HOST }

# 参数处理
$argsList = $args
$modeSet = $false
$i = 0
while ($i -lt $argsList.Count) {
    $arg = $argsList[$i]
    if ($arg -match "^-") {
        # 选项
        switch ($arg) {
            "--lan" {
                $FRONTEND_BIND_HOST = "0.0.0.0"
                $i++
            }
            "--host" {
                if ($i + 1 -ge $argsList.Count) {
                    Write-Error "错误: --host 需要指定监听地址"
                    exit 1
                }
                $FRONTEND_BIND_HOST = $argsList[$i+1]
                $i += 2
            }
            "-h" {
                Write-Output @"
用法: .\bootstrap.ps1 [all|fe|be] [options]
  all  - 启动前后端 (默认)
  fe   - 仅启动前端 (Vite dev server)
  be   - 仅启动后端 (Go server)

前端选项:
  --lan          允许同一局域网设备访问前端，等同于 --host 0.0.0.0
  --host <host>  指定 Vite dev server 监听地址
"@
                exit 0
            }
            "--help" {
                # 同上
                Write-Output "用法: ..."  # 省略重复，可写相同内容
                exit 0
            }
            default {
                Write-Error "错误: 未知参数 $arg"
                exit 1
            }
        }
    } else {
        # 非选项参数，视为 MODE
        if (-not $modeSet) {
            $MODE = $arg
            $modeSet = $true
            $i++
        } else {
            Write-Error "错误: 多余的参数 $arg"
            exit 1
        }
    }
}

# 检查 pnpm
function Test-Pnpm {
    if (-not (Get-Command pnpm -ErrorAction SilentlyContinue)) {
        Write-Error "错误: 未找到 pnpm，请先安装 pnpm"
        exit 1
    }
}

function Test-Go {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        Write-Error "错误: 未找到 go，请先安装 Go"
        exit 1
    }
}

# 根据模式执行
switch ($MODE) {
    "fe" {
        Write-Output "==> Denova 前端开发服务启动"
        Write-Output "  前端地址: $FRONTEND_URL"
        if ($FRONTEND_BIND_HOST -eq "0.0.0.0") {
            $lan = Detect-LanAddress
            if ($lan) {
                Write-Output "  局域网地址: http://${lan}:${FRONTEND_PORT}"
            } else {
                Write-Output "  局域网地址: http://<本机局域网IP>:${FRONTEND_PORT}"
            }
        } elseif ($FRONTEND_BIND_HOST) {
            Write-Output "  监听地址: $FRONTEND_BIND_HOST"
        }
        Write-Output ""

        Test-Pnpm

        if (-not (Test-Path "web/node_modules")) {
            Write-Output "==> 安装前端依赖"
            Push-Location web
            & pnpm install
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
            Pop-Location
        }

        Write-Output "  按 Ctrl+C 停止服务"
        $env:DENOVA_BACKEND_PORT = $BACKEND_PORT
        $env:DENOVA_FRONTEND_PORT = $FRONTEND_PORT
        Push-Location web
        if ($FRONTEND_BIND_HOST) {
            & pnpm dev --host $FRONTEND_BIND_HOST --port $FRONTEND_PORT
        } else {
            & pnpm dev --port $FRONTEND_PORT
        }
        Pop-Location
    }

    "be" {
        Write-Output "==> Denova 后端开发服务启动"
        Write-Output "  后端地址: $BACKEND_URL"
        Write-Output ""

        Test-Go

        Write-Output "==> 拉取 Go 依赖"
        & go mod tidy
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

        Write-Output "  按 Ctrl+C 停止服务"
        & go run ./cmd/denova --dev-mode --no-open --port $BACKEND_PORT --frontend-port $FRONTEND_PORT
    }

    "all" {
        Write-Output "==> Denova 开发服务启动"
        Write-Output "  前端地址: $FRONTEND_URL"
        Write-Output "  后端地址: $BACKEND_URL"
        Write-Output ""

        Test-Pnpm
        Test-Go

        if (-not (Test-Path "web/node_modules")) {
            Write-Output "==> 安装前端依赖"
            Push-Location web
            & pnpm install
            if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
            Pop-Location
        }

        Write-Output "==> 拉取 Go 依赖"
        & go mod tidy
        if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

        Write-Output "==> 启动前后端"
        Write-Output "  按 Ctrl+C 停止服务"
        Write-Output ""
        & go run ./cmd/denova --dev --dev-mode --no-open --port $BACKEND_PORT --frontend-port $FRONTEND_PORT
    }

    default {
        Write-Error "错误: 未知模式 '$MODE'"
        exit 1
    }
}