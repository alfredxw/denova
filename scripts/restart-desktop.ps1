#requires -Version 5.1
[CmdletBinding()]
param(
    [switch]$Rebuild
)

$ErrorActionPreference = 'Stop'
if ($env:OS -ne 'Windows_NT') {
    throw 'Run this script in native Windows PowerShell.'
}

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$buildDirectory = Join-Path $repoRoot 'desktop\build'
$appPaths = @(
    (Join-Path $buildDirectory 'Denova.exe'),
    (Join-Path $repoRoot 'desktop\dist\windows-amd64\Denova.exe')
)

foreach ($name in @('go.exe', 'node.exe', 'pnpm.cmd')) {
    if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
        throw "Missing required command: $name. Install it and add it to PATH first."
    }
}

New-Item -ItemType Directory -Path $buildDirectory -Force | Out-Null
# Keep the file; the exclusive handle, not its existence, is the lock.
$lockPath = Join-Path $buildDirectory '.restart-desktop-windows.lock'
try {
    $restartLock = [System.IO.File]::Open($lockPath, 'OpenOrCreate', 'ReadWrite', 'None')
}
catch {
    throw "Another desktop restart is in progress, or the build directory is not writable: $lockPath"
}

Push-Location $repoRoot
try {
    $snapshot = @(Get-CimInstance Win32_Process)
    $apps = @($snapshot | Where-Object { $_.ExecutablePath -and $_.ExecutablePath -in $appPaths })
    foreach ($app in $apps) {
        $process = Get-Process -Id $app.ProcessId -ErrorAction SilentlyContinue
        if (-not $process -or $process.Path -notin $appPaths) { continue }
        $launcher = $snapshot | Where-Object {
            $_.ProcessId -eq $app.ParentProcessId -and $_.Name -eq 'node.exe' -and
            $_.CommandLine -match '(?:^|[\s"/\\])dev\.mjs(?:["\s]|$)'
        } | ForEach-Object { Get-Process -Id $_.ProcessId -ErrorAction SilentlyContinue }

        Write-Host "Requesting normal desktop quit (PID $($process.Id)). Confirm in Denova if prompted."
        Write-Host 'For an older build, choose Options > Quit Denova manually if it stays open.'
        # The target PID prevents another checkout or installed app from quitting.
        & $app.ExecutablePath "--quit-pid=$($process.Id)"
        if ($LASTEXITCODE -ne 0) { throw "Desktop quit request failed with code $LASTEXITCODE." }
        if (-not $process.WaitForExit(120000)) {
            throw 'Desktop is still running. Quit Denova and rerun this script; no process was forcibly terminated.'
        }
        # dev.mjs owns Vite and closes it after the desktop exits normally.
        if ($launcher -and -not $launcher.WaitForExit(15000)) {
            throw 'The previous desktop launcher is still closing. Wait for it to exit and rerun this script.'
        }
    }

    if (-not (Test-Path -LiteralPath (Join-Path $repoRoot 'web\node_modules\.modules.yaml'))) {
        Write-Host 'Installing frontend dependencies.'
        & pnpm.cmd --dir web install --frozen-lockfile
        if ($LASTEXITCODE -ne 0) { throw "Dependency installation failed with code $LASTEXITCODE." }
    }

    $missingResources = @(
        'backend\denova.exe', 'backend\tools\rg.exe', 'backend\skills',
        'backend\config.toml', 'backend\LICENSE', 'backend\CHANGELOG.md',
        'backend\licenses\ripgrep\LICENSE-MIT', 'backend\licenses\ripgrep\UNLICENSE',
        'icon.png', 'tray.png'
    ) | Where-Object {
        -not (Test-Path -LiteralPath (Join-Path $buildDirectory $_))
    }
    if ($Rebuild -or $missingResources) {
        Write-Host 'Building the Windows desktop backend and resources.'
        $previousTarget = $env:DENOVA_DESKTOP_TARGET
        $previousFrontend = $env:DENOVA_RELEASE_FRONTEND_DIR
        try {
            $env:DENOVA_DESKTOP_TARGET = 'win32-x64'
            $env:DENOVA_RELEASE_FRONTEND_DIR = $null
            & pnpm.cmd --dir desktop build:backend
            if ($LASTEXITCODE -ne 0) { throw "Desktop resource build failed with code $LASTEXITCODE." }
        }
        finally {
            $env:DENOVA_DESKTOP_TARGET = $previousTarget
            $env:DENOVA_RELEASE_FRONTEND_DIR = $previousFrontend
        }
    }

    Write-Host "Starting Denova Desktop with frontend hot updates: $repoRoot"
    Write-Host 'Keep this terminal open. Use Options > Quit Denova to exit.'
    # The next invocation must be able to request a restart while this one waits.
    $restartLock.Dispose()
    & pnpm.cmd --dir desktop dev:hmr
    if ($LASTEXITCODE -ne 0) { throw "Desktop development launcher exited with code $LASTEXITCODE." }
}
finally {
    $restartLock.Dispose()
    Pop-Location
}
