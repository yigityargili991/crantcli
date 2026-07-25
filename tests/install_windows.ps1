$ErrorActionPreference = "Stop"
Set-StrictMode -Version 2.0

$repositoryRoot = Split-Path -Parent $PSScriptRoot
$testRoot = Join-Path ([IO.Path]::GetTempPath()) "crantcli-installer-test-$([Guid]::NewGuid().ToString("N"))"
$fixtures = Join-Path $testRoot "fixtures"
$originalProcessPath = $env:Path
$originalUserPath = [Environment]::GetEnvironmentVariable("Path", "User")
$originalArchitecture = $env:PROCESSOR_ARCHITECTURE
$originalWowArchitecture = $env:PROCESSOR_ARCHITEW6432
$originalInstallDirectory = $env:CRANTCLI_INSTALL_DIR
$originalVersion = $env:CRANTCLI_VERSION
$originalSkipChecksum = $env:CRANTCLI_SKIP_CHECKSUM
$originalGithubToken = $env:CRANTCLI_GITHUB_TOKEN
$env:CRANTCLI_GITHUB_TOKEN = $null
$global:CrantCliInstallerTestFixtures = $fixtures
$global:CrantCliInstallerRequestedUris = [Collections.Generic.List[string]]::new()

function global:Invoke-WebRequest {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string]$OutFile,
        [switch]$UseBasicParsing
    )

    $global:CrantCliInstallerRequestedUris.Add($Uri)
    $fixtureName = [IO.Path]::GetFileName(([Uri]$Uri).AbsolutePath)
    $fixture = Join-Path $global:CrantCliInstallerTestFixtures $fixtureName
    if (-not (Test-Path -LiteralPath $fixture)) {
        throw "unexpected download: $Uri"
    }
    Copy-Item -LiteralPath $fixture -Destination $OutFile
}

function Assert-Equal {
    param(
        [Parameter(Mandatory = $true)]$Expected,
        [Parameter(Mandatory = $true)]$Actual,
        [Parameter(Mandatory = $true)][string]$Message
    )

    if ($Expected -ne $Actual) {
        throw "$Message (expected '$Expected', got '$Actual')"
    }
}

function Write-Checksums {
    param([switch]$Invalid)

    $lines = foreach ($architecture in @("amd64", "arm64")) {
        $asset = "crant_type_look-windows-$architecture.exe"
        $hash = if ($Invalid) {
            ("0" * 64) -join ""
        }
        else {
            (Get-FileHash -LiteralPath (Join-Path $fixtures $asset) -Algorithm SHA256).Hash.ToLowerInvariant()
        }
        "$hash  $asset"
    }
    Set-Content -LiteralPath (Join-Path $fixtures "checksums.txt") -Value $lines
}

function Test-Install {
    param(
        [Parameter(Mandatory = $true)][ValidateSet("AMD64", "ARM64")][string]$Architecture,
        [Parameter(Mandatory = $true)][string]$Version
    )

    $assetArchitecture = $Architecture.ToLowerInvariant()
    $installDirectory = Join-Path $testRoot "install-$assetArchitecture"
    $env:PROCESSOR_ARCHITECTURE = $Architecture
    $env:PROCESSOR_ARCHITEW6432 = $null
    $env:CRANTCLI_INSTALL_DIR = $installDirectory
    $env:CRANTCLI_VERSION = $Version
    $global:CrantCliInstallerRequestedUris.Clear()

    & (Join-Path $repositoryRoot "install.ps1")

    $installedFile = Join-Path $installDirectory "crantcli.exe"
    Assert-Equal `
        (Get-Content -LiteralPath (Join-Path $fixtures "crant_type_look-windows-$assetArchitecture.exe") -Raw) `
        (Get-Content -LiteralPath $installedFile -Raw) `
        "$Architecture asset was not installed"
    if (-not (($env:Path -split ";") -contains $installDirectory)) {
        throw "$installDirectory was not added to the current PATH"
    }

    $releasePath = if ($Version -eq "latest") {
        "/releases/latest/download/"
    }
    else {
        "/releases/download/$Version/"
    }
    if (-not ($global:CrantCliInstallerRequestedUris[0].Contains($releasePath))) {
        throw "installer used the wrong release URL for $Version"
    }
}

New-Item -ItemType Directory -Path $fixtures | Out-Null
try {
    Set-Content -LiteralPath (Join-Path $fixtures "crant_type_look-windows-amd64.exe") -Value "amd64 fixture" -NoNewline
    Set-Content -LiteralPath (Join-Path $fixtures "crant_type_look-windows-arm64.exe") -Value "arm64 fixture" -NoNewline
    Write-Checksums

    Test-Install -Architecture "AMD64" -Version "latest"
    Test-Install -Architecture "ARM64" -Version "v1.2.3"

    Write-Checksums -Invalid
    $env:PROCESSOR_ARCHITECTURE = "AMD64"
    $env:CRANTCLI_INSTALL_DIR = Join-Path $testRoot "checksum-failure"
    $env:CRANTCLI_VERSION = "latest"
    try {
        & (Join-Path $repositoryRoot "install.ps1")
        throw "installer accepted a binary with an invalid checksum"
    }
    catch {
        if ($_.Exception.Message -notlike "*checksum mismatch*") {
            throw
        }
    }
    if (Test-Path -LiteralPath (Join-Path $env:CRANTCLI_INSTALL_DIR "crantcli.exe")) {
        throw "installer copied a binary after checksum verification failed"
    }

    Write-Host "Windows installer tests passed"
}
finally {
    $env:Path = $originalProcessPath
    [Environment]::SetEnvironmentVariable("Path", $originalUserPath, "User")
    $env:PROCESSOR_ARCHITECTURE = $originalArchitecture
    $env:PROCESSOR_ARCHITEW6432 = $originalWowArchitecture
    $env:CRANTCLI_INSTALL_DIR = $originalInstallDirectory
    $env:CRANTCLI_VERSION = $originalVersion
    $env:CRANTCLI_SKIP_CHECKSUM = $originalSkipChecksum
    $env:CRANTCLI_GITHUB_TOKEN = $originalGithubToken
    Remove-Item function:global:Invoke-WebRequest -ErrorAction SilentlyContinue
    Remove-Variable CrantCliInstallerTestFixtures -Scope Global -ErrorAction SilentlyContinue
    Remove-Variable CrantCliInstallerRequestedUris -Scope Global -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $testRoot) {
        Remove-Item -LiteralPath $testRoot -Recurse -Force
    }
}
