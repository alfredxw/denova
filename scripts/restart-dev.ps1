[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..')).TrimEnd('\')
$webNodeModules = Join-Path $repoRoot 'web\node_modules'
$stateDirectory = Join-Path $repoRoot 'log'
$statePath = Join-Path $stateDirectory 'dev-windows.json'
$localGo = Join-Path $env:USERPROFILE '.local\go\bin\go.exe'
$corepackPnpm = Join-Path $env:ProgramFiles 'nodejs\node_modules\corepack\shims\pnpm.cmd'

function Get-ProcessSnapshot {
    return @(Get-CimInstance Win32_Process | Select-Object ProcessId, ParentProcessId, Name, CommandLine)
}

function Get-DescendantProcessIds {
    param(
        [Parameter(Mandatory)]
        [int[]]$RootProcessIds,
        [Parameter(Mandatory)]
        [object[]]$Snapshot
    )

    $selected = [System.Collections.Generic.HashSet[int]]::new()
    $queue = [System.Collections.Generic.Queue[int]]::new()
    foreach ($processId in $RootProcessIds) {
        if ($selected.Add($processId)) {
            $queue.Enqueue($processId)
        }
    }

    while ($queue.Count -gt 0) {
        $parentId = $queue.Dequeue()
        foreach ($child in $Snapshot | Where-Object { $_.ParentProcessId -eq $parentId }) {
            if ($selected.Add([int]$child.ProcessId)) {
                $queue.Enqueue([int]$child.ProcessId)
            }
        }
    }

    return @($selected)
}

function Find-RepoViteProcessIds {
    param(
        [Parameter(Mandatory)]
        [object[]]$Snapshot
    )

    return @($Snapshot | Where-Object {
        $_.Name -eq 'node.exe' -and
        $_.CommandLine -and
        $_.CommandLine.Replace('/', '\').IndexOf($webNodeModules, [System.StringComparison]::OrdinalIgnoreCase) -ge 0 -and
        $_.CommandLine -match '[\\/]vite[\\/]bin[\\/]vite\.js'
    } | ForEach-Object { [int]$_.ProcessId })
}

function Get-FrontendBranchProcessIds {
    param(
        [Parameter(Mandatory)]
        [int[]]$ViteProcessIds,
        [Parameter(Mandatory)]
        [object[]]$Snapshot
    )

    $selected = [System.Collections.Generic.HashSet[int]]::new()
    foreach ($viteProcessId in $ViteProcessIds) {
        # Preserve Vite's launchers as well as its worker processes so the
        # existing frontend keeps its original lifecycle intact.
        foreach ($processId in Get-DescendantProcessIds -RootProcessIds @($viteProcessId) -Snapshot $Snapshot) {
            [void]$selected.Add([int]$processId)
        }

        $current = $Snapshot | Where-Object { $_.ProcessId -eq $viteProcessId } | Select-Object -First 1
        while ($current -and $current.Name -notin @('denova.exe', 'go.exe')) {
            [void]$selected.Add([int]$current.ProcessId)
            $current = $Snapshot | Where-Object { $_.ProcessId -eq $current.ParentProcessId } | Select-Object -First 1
        }
    }

    return @($selected)
}

function Find-RepoDevProcessIds {
    param(
        [Parameter(Mandatory)]
        [object[]]$Snapshot
    )

    $roots = [System.Collections.Generic.HashSet[int]]::new()

    if (Test-Path -LiteralPath $statePath) {
        try {
            $state = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
            if ([string]::Equals([string]$state.repo_root, $repoRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
                $recorded = $Snapshot | Where-Object { $_.ProcessId -eq [int]$state.process_id } | Select-Object -First 1
                if ($recorded -and $recorded.Name -eq 'go.exe' -and $recorded.CommandLine -match 'run\s+\.?[\\/]cmd[\\/]denova') {
                    [void]$roots.Add([int]$recorded.ProcessId)
                }
            }
        }
        catch {
            Write-Warning 'Ignoring an invalid Windows dev state file. / Windows dev 状态文件无效，已忽略。'
        }
    }

    $viteProcessIds = @(Find-RepoViteProcessIds -Snapshot $Snapshot)
    foreach ($viteProcessId in $viteProcessIds) {
        $vite = $Snapshot | Where-Object { $_.ProcessId -eq $viteProcessId } | Select-Object -First 1
        $current = $vite
        while ($current) {
            if ($current.Name -in @('denova.exe', 'go.exe')) {
                [void]$roots.Add([int]$current.ProcessId)
            }
            $current = $Snapshot | Where-Object { $_.ProcessId -eq $current.ParentProcessId } | Select-Object -First 1
        }
    }

    return @($roots)
}

function Stop-RepoDevProcesses {
    $snapshot = Get-ProcessSnapshot
    $viteProcessIds = @(Find-RepoViteProcessIds -Snapshot $snapshot)
    $frontendProcessIds = if ($viteProcessIds.Count -gt 0) {
        @(Get-FrontendBranchProcessIds -ViteProcessIds $viteProcessIds -Snapshot $snapshot)
    }
    else {
        @()
    }
    $rootProcessIds = @(Find-RepoDevProcessIds -Snapshot $snapshot)
    if ($rootProcessIds.Count -eq 0) {
        Write-Host 'No running Windows dev instance found. / 未发现正在运行的 Windows dev 实例。'
        return
    }

    $allProcessIds = @(Get-DescendantProcessIds -RootProcessIds $rootProcessIds -Snapshot $snapshot | Where-Object {
        $_ -notin $frontendProcessIds
    })
    Write-Host "Stopping the current Windows dev instance (PID: $($rootProcessIds -join ', ')). / 正在停止当前 Windows dev 实例。"
    if (Test-Path -LiteralPath $statePath) {
        try {
            $state = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
            if ([string]::Equals([string]$state.repo_root, $repoRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
                $state | Add-Member -NotePropertyName stop_requested -NotePropertyValue $true -Force
                $state | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding utf8
            }
        }
        catch {
            Write-Warning 'Could not mark the previous dev instance as intentionally stopped. / 无法标记旧 dev 实例为主动停止。'
        }
    }
    foreach ($processId in $allProcessIds | Sort-Object -Descending) {
        Stop-Process -Id $processId -Force -ErrorAction SilentlyContinue
    }
}

function Resolve-RequiredCommand {
    param(
        [Parameter(Mandatory)]
        [string]$Name,
        [string]$FallbackPath
    )

    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if ($command) {
        return $command.Source
    }
    if ($FallbackPath -and (Test-Path -LiteralPath $FallbackPath)) {
        return $FallbackPath
    }
    throw "Missing required command: $Name. Install the Windows dependency first. / 缺少必要命令：$Name，请先安装 Windows 依赖。"
}

$goExecutable = Resolve-RequiredCommand -Name 'go.exe' -FallbackPath $localGo
$goBin = Split-Path -Parent $goExecutable
$env:Path = "$goBin;$env:Path"

Stop-RepoDevProcesses
$frontendAlreadyRunning = @(Find-RepoViteProcessIds -Snapshot (Get-ProcessSnapshot)).Count -gt 0

$denovaArguments = @('run', './cmd/denova')
if ($frontendAlreadyRunning) {
    Write-Host 'Keeping the existing Vite frontend and restarting only the backend.'
}
else {
    $pnpmExecutable = Resolve-RequiredCommand -Name 'pnpm.cmd' -FallbackPath $corepackPnpm
    $pnpmBin = Split-Path -Parent $pnpmExecutable
    $env:Path = "$pnpmBin;$env:Path"
    $denovaArguments += '--dev'
}
$denovaArguments += @('--dev-mode', '--no-open')

New-Item -ItemType Directory -Path $stateDirectory -Force | Out-Null
Write-Host 'Starting Denova in the native Windows dev environment. / 正在原生 Windows dev 环境启动 Denova。'
Write-Host "Repository / 仓库: $repoRoot"

$goProcess = Start-Process `
    -FilePath $goExecutable `
    -ArgumentList $denovaArguments `
    -WorkingDirectory $repoRoot `
    -NoNewWindow `
    -PassThru

@{
    process_id = $goProcess.Id
    repo_root = $repoRoot
    stop_requested = $false
} | ConvertTo-Json | Set-Content -LiteralPath $statePath -Encoding utf8

$stopRequested = $false
try {
    $goProcess.WaitForExit()
    $exitCode = $goProcess.ExitCode
}
finally {
    if (Test-Path -LiteralPath $statePath) {
        $currentState = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
        if ([int]$currentState.process_id -eq $goProcess.Id) {
            $stopRequested = [bool]$currentState.stop_requested
            Remove-Item -LiteralPath $statePath -Force
        }
    }
}

if ($exitCode -ne 0 -and -not $stopRequested) {
    throw "Windows dev process exited with code $exitCode. / Windows dev 进程异常退出，退出码：$exitCode。"
}
