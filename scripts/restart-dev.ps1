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

function Test-DenovaGoRunProcess {
    param(
        [Parameter(Mandatory)]
        [object]$Process
    )

    return $Process.Name -eq 'go.exe' -and
        $Process.CommandLine -and
        $Process.CommandLine -match '(?i)(?:^|\s)run\s+(?:"[^"]*[\\/]|[^\s"]*[\\/])?cmd[\\/]denova(?:"?)(?:\s|$)'
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
    $processById = @{}
    foreach ($process in $Snapshot) {
        $processById[[int]$process.ProcessId] = $process
    }

    foreach ($record in $StateRecords) {
        $recorded = $processById[[int]$record.State.process_id]
        if ($recorded -and (Test-DenovaGoRunProcess -Process $recorded)) {
            [void]$roots.Add([int]$recorded.ProcessId)
        }
    }

    # A live backend plus its go run parent identifies backend-only bootstrap
    # trees that have neither a state file nor a Vite child.
    foreach ($backend in $Snapshot | Where-Object {
        $_.Name -eq 'denova.exe' -and
        $_.CommandLine -and
        $_.CommandLine -match '(?i)(?:^|\s)--dev-mode(?:\s|$)'
    }) {
        $root = $null
        $current = $processById[[int]$backend.ParentProcessId]
        while ($current -and (Test-DenovaGoRunProcess -Process $current)) {
            $root = $current
            $current = $processById[[int]$current.ParentProcessId]
        }
        if ($root) {
            [void]$roots.Add([int]$root.ProcessId)
        }
    }

    foreach ($viteProcessId in Find-RepoViteProcessIds -Snapshot $Snapshot) {
        $current = $processById[[int]$viteProcessId]
        $root = $null
        while ($current) {
            if ($current.Name -in @('denova.exe', 'go.exe')) {
                $root = $current
            }
            $current = $processById[[int]$current.ParentProcessId]
        }
        if ($root) {
            [void]$roots.Add([int]$root.ProcessId)
        }
        else {
            [void]$roots.Add([int]$viteProcessId)
        }
    }

    return @($roots)
}

function Stop-RepoDevProcesses {
    $snapshot = Get-ProcessSnapshot
    $stateRecords = @(Get-RepoDevStateRecords)
    $rootProcessIds = @(Find-RepoDevProcessIds -Snapshot $snapshot -StateRecords $stateRecords)
    if ($rootProcessIds.Count -eq 0) {
        Write-Host 'No running Windows development instance found.'
        return
    }

    $allProcessIds = @(Get-DescendantProcessIds -RootProcessIds $rootProcessIds -Snapshot $snapshot)
    Write-Host "Stopping Denova development process trees (root PID: $($rootProcessIds -join ', '))."
    foreach ($record in $stateRecords) {
        try {
            $record.State | Add-Member -NotePropertyName stop_requested -NotePropertyValue $true -Force
            $record.State | ConvertTo-Json | Set-Content -LiteralPath $record.Path -Encoding utf8
        }
        catch {
            Write-Warning "Could not mark the previous development instance as intentionally stopped: $($record.Path)"
        }
    }
    $stoppedProcesses = @()
    foreach ($processId in $allProcessIds | Sort-Object -Descending) {
        $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
        if ($process) {
            Stop-Process -InputObject $process -Force
            $stoppedProcesses += $process
        }
    }
    if ($stoppedProcesses.Count -gt 0) {
        $stoppedProcesses | Wait-Process -ErrorAction SilentlyContinue
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
