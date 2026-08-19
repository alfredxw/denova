[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

$repoRoot = [System.IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..')).TrimEnd('\')
$webNodeModules = Join-Path $repoRoot 'web\node_modules'
$stateDirectory = Join-Path $repoRoot 'log'
$stateFilePattern = 'dev-windows-*.json'
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

function Get-RepoDevStateRecords {
    if (-not (Test-Path -LiteralPath $stateDirectory)) {
        return @()
    }

    $records = @()
    foreach ($file in Get-ChildItem -LiteralPath $stateDirectory -Filter $stateFilePattern -File) {
        try {
            $state = Get-Content -LiteralPath $file.FullName -Raw | ConvertFrom-Json
            if ([string]::Equals([string]$state.repo_root, $repoRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
                $records += [pscustomobject]@{
                    Path = $file.FullName
                    State = $state
                }
            }
        }
        catch {
            Write-Warning "Ignoring invalid Windows development state file: $($file.FullName)"
        }
    }

    return @($records)
}

function Find-RepoDevProcessIds {
    param(
        [Parameter(Mandatory)]
        [object[]]$Snapshot,
        [Parameter(Mandatory)]
        [AllowEmptyCollection()]
        [object[]]$StateRecords
    )

    $roots = [System.Collections.Generic.HashSet[int]]::new()

    foreach ($record in $StateRecords) {
        $recorded = $Snapshot | Where-Object { $_.ProcessId -eq [int]$record.State.process_id } | Select-Object -First 1
        if ($recorded -and $recorded.Name -eq 'go.exe' -and $recorded.CommandLine -match 'run\s+\.?[\\/]cmd[\\/]denova') {
            [void]$roots.Add([int]$recorded.ProcessId)
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
    $stateRecords = @(Get-RepoDevStateRecords)
    $backendProcessIds = @(Find-RepoDevProcessIds -Snapshot $snapshot -StateRecords $stateRecords)
    $viteProcessIds = @(Find-RepoViteProcessIds -Snapshot $snapshot)
    $rootProcessIds = @($backendProcessIds + $viteProcessIds | Select-Object -Unique)
    if ($rootProcessIds.Count -eq 0) {
        Write-Host 'No running Windows development instance found.'
        return
    }

    $allProcessIds = @(Get-DescendantProcessIds -RootProcessIds $rootProcessIds -Snapshot $snapshot)
    Write-Host "Stopping the current Windows development instance (PID: $($rootProcessIds -join ', '))."
    foreach ($record in $stateRecords) {
        try {
            $record.State | Add-Member -NotePropertyName stop_requested -NotePropertyValue $true -Force
            $record.State | ConvertTo-Json | Set-Content -LiteralPath $record.Path -Encoding utf8
        }
        catch {
            Write-Warning "Could not mark the previous development instance as intentionally stopped: $($record.Path)"
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
    throw "Missing required command: $Name. Install the Windows dependency first."
}

$goExecutable = Resolve-RequiredCommand -Name 'go.exe' -FallbackPath $localGo
$goBin = Split-Path -Parent $goExecutable
$env:Path = "$goBin;$env:Path"
$pnpmExecutable = Resolve-RequiredCommand -Name 'pnpm.cmd' -FallbackPath $corepackPnpm
$pnpmBin = Split-Path -Parent $pnpmExecutable
$env:Path = "$pnpmBin;$env:Path"

Stop-RepoDevProcesses
$denovaArguments = @('run', './cmd/denova', '--dev', '--dev-mode', '--no-open')

New-Item -ItemType Directory -Path $stateDirectory -Force | Out-Null
Write-Host 'Starting Denova in the native Windows development environment.'
Write-Host "Repository: $repoRoot"

$goProcess = Start-Process `
    -FilePath $goExecutable `
    -ArgumentList $denovaArguments `
    -WorkingDirectory $repoRoot `
    -NoNewWindow `
    -PassThru

$statePath = Join-Path $stateDirectory "dev-windows-$($goProcess.Id).json"
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
        try {
            $currentState = Get-Content -LiteralPath $statePath -Raw | ConvertFrom-Json
            $stopRequested = [bool]$currentState.stop_requested
        }
        catch {
            Write-Warning "Could not read the Windows development state file: $statePath"
        }
        finally {
            Remove-Item -LiteralPath $statePath -Force
        }
    }
}

if ($exitCode -ne 0 -and -not $stopRequested) {
    throw "Windows development process exited with code $exitCode."
}
