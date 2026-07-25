<#
.SYNOPSIS
    Builds Open Monitoring into a single self-contained executable.

.DESCRIPTION
    Three components go into the app, and this script produces all of them in
    one pass:

      1. lhm-bridge  - the C# sensor helper, published self-contained so the
                       machine running the app needs no .NET installed.
      2. PresentMon  - Intel's frame-time tool, downloaded from its official
                       GitHub release if it is not already present.
      3. the app     - Go + Svelte, with both helpers compiled in via go:embed.

    The helpers are staged into internal/sidecar/bin before the Go build so the
    embed directive picks them up. The result is build/bin/open-monitoring.exe
    with nothing beside it.

.PARAMETER SkipHelpers
    Reuse the helpers already staged in internal/sidecar/bin and go straight to
    the app build. Useful when iterating on Go or frontend code.

.PARAMETER PresentMonVersion
    Which PresentMon release to download when one is not already staged.

.EXAMPLE
    .\build.ps1
#>
[CmdletBinding()]
param(
    [switch]$SkipHelpers,
    [string]$PresentMonVersion = '2.5.1'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$root  = $PSScriptRoot
$stage = Join-Path $root 'internal\sidecar\bin'

function Write-Step($message) {
    Write-Host "==> $message" -ForegroundColor Cyan
}

function Resolve-Dotnet {
    # A machine can have several dotnet installations, and the one on PATH is
    # commonly the runtime-only redistributable — it resolves fine but cannot
    # build. Pick the first candidate that actually reports an SDK.
    $candidates = @()

    $onPath = Get-Command dotnet -ErrorAction SilentlyContinue
    if ($onPath) { $candidates += $onPath.Source }

    # The SDK is frequently installed user-scope, which is not always on PATH.
    $userScope = Join-Path $env:LOCALAPPDATA 'Microsoft\dotnet\dotnet.exe'
    if (Test-Path $userScope) { $candidates += $userScope }

    foreach ($candidate in $candidates) {
        $sdks = & $candidate --list-sdks 2>$null
        if ($LASTEXITCODE -eq 0 -and $sdks) { return $candidate }
    }

    throw 'The .NET 8 SDK is required to build the sensor bridge. See https://dotnet.microsoft.com/download'
}

function Build-Bridge {
    Write-Step 'Publishing the sensor bridge (lhm-bridge)'

    $dotnet  = Resolve-Dotnet
    $project = Join-Path $root 'tools\lhm-bridge'
    $publish = Join-Path $project 'publish'

    & $dotnet publish $project -c Release -o $publish
    if ($LASTEXITCODE -ne 0) { throw "dotnet publish failed with exit code $LASTEXITCODE" }

    Copy-Item (Join-Path $publish 'lhm-bridge.exe') $stage -Force
}

function Get-PresentMon {
    Write-Step 'Resolving PresentMon'

    # Prefer a copy already in the repo so the build works offline.
    $local = Join-Path $root 'tools\presentmon\PresentMon.exe'
    if (Test-Path $local) {
        Copy-Item $local $stage -Force
        Write-Host "    using $local"
        return
    }

    $url = "https://github.com/GameTechDev/PresentMon/releases/download/v$PresentMonVersion/PresentMon-$PresentMonVersion-x64.exe"
    Write-Host "    downloading $url"

    try {
        New-Item -ItemType Directory -Force -Path (Split-Path $local) | Out-Null
        Invoke-WebRequest -Uri $url -OutFile $local -UseBasicParsing
        Copy-Item $local $stage -Force
    }
    catch {
        # FPS is one feature among many; a failed download should not stop a
        # build that is otherwise fine.
        Write-Warning "PresentMon could not be downloaded ($($_.Exception.Message))."
        Write-Warning 'The app will build without frame-rate support.'
    }
}

function Build-App {
    Write-Step 'Building the app (wails)'

    if (-not (Get-Command wails -ErrorAction SilentlyContinue)) {
        throw 'The Wails CLI is required: go install github.com/wailsapp/wails/v2/cmd/wails@latest'
    }

    & wails build
    if ($LASTEXITCODE -ne 0) { throw "wails build failed with exit code $LASTEXITCODE" }
}

New-Item -ItemType Directory -Force -Path $stage | Out-Null

if ($SkipHelpers) {
    Write-Step 'Skipping helpers; using what is already staged'
}
else {
    Build-Bridge
    Get-PresentMon
}

Build-App

$exe = Join-Path $root 'build\bin\open-monitoring.exe'
if (Test-Path $exe) {
    $sizeMb = [math]::Round((Get-Item $exe).Length / 1MB, 1)
    Write-Host ''
    Write-Host "Built $exe ($sizeMb MB)" -ForegroundColor Green
}
