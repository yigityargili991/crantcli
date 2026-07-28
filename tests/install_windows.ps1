$ErrorActionPreference = "Stop"
Set-StrictMode -Version 2.0

$repositoryRoot = Split-Path -Parent $PSScriptRoot
$testRoot = Join-Path ([IO.Path]::GetTempPath()) "crantcli-installer-test-$([Guid]::NewGuid().ToString("N"))"
$fixtures = Join-Path $testRoot "fixtures"
$fakeBin = Join-Path $testRoot "bin"
$originalProcessPath = $env:Path
$originalUserPath = [Environment]::GetEnvironmentVariable("Path", "User")
$originalArchitecture = $env:PROCESSOR_ARCHITECTURE
$originalWowArchitecture = $env:PROCESSOR_ARCHITEW6432
$originalInstallDirectory = $env:CRANTCLI_INSTALL_DIR
$originalVersion = $env:CRANTCLI_VERSION
$originalSkipChecksum = $env:CRANTCLI_SKIP_CHECKSUM
$originalRequireSignature = $env:CRANTCLI_REQUIRE_SIGNATURE
$originalGithubToken = $env:CRANTCLI_GITHUB_TOKEN
$originalCosignFail = $env:CRANTCLI_TEST_COSIGN_FAIL
$env:CRANTCLI_GITHUB_TOKEN = $null
$env:CRANTCLI_REQUIRE_SIGNATURE = $null
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
    $asset = "crant_type_look-windows-$assetArchitecture.exe"
    $installDirectory = Join-Path $testRoot "install-$assetArchitecture"
    $installedFile = Join-Path $installDirectory "crantcli.exe"
    New-Item -ItemType Directory -Path $installDirectory | Out-Null
    Set-Content -LiteralPath $installedFile -Value "old fixture" -NoNewline
    $env:PROCESSOR_ARCHITECTURE = $Architecture
    $env:PROCESSOR_ARCHITEW6432 = $null
    $env:CRANTCLI_INSTALL_DIR = $installDirectory
    $env:CRANTCLI_VERSION = $Version
    $global:CrantCliInstallerRequestedUris.Clear()

    & (Join-Path $repositoryRoot "install.ps1")

    Assert-Equal `
        (Get-Content -LiteralPath (Join-Path $fixtures "crant_type_look-windows-$assetArchitecture.exe") -Raw) `
        (Get-Content -LiteralPath $installedFile -Raw) `
        "$Architecture asset was not installed"
    if (Test-Path -LiteralPath "${installedFile}.old") {
        throw "installer left an unlocked backup at ${installedFile}.old"
    }
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
    if (-not ($global:CrantCliInstallerRequestedUris | Where-Object { $_.EndsWith("$asset.sigstore.json") })) {
        throw "installer did not download the signature bundle for $asset"
    }
}

New-Item -ItemType Directory -Path $fixtures, $fakeBin | Out-Null
try {
    foreach ($architecture in @("amd64", "arm64")) {
        $asset = "crant_type_look-windows-$architecture.exe"
        Set-Content -LiteralPath (Join-Path $fixtures $asset) -Value "$architecture fixture" -NoNewline
        Set-Content -LiteralPath (Join-Path $fixtures "$asset.sigstore.json") -Value '{"fixture":"signature"}' -NoNewline
    }
    Set-Content -LiteralPath (Join-Path $fakeBin "cosign.cmd") -Value @(
        '@echo off',
        'if "%CRANTCLI_TEST_COSIGN_FAIL%"=="1" exit /b 1',
        'exit /b 0'
    )
    $env:Path = "$fakeBin;$env:Path"

    $installerSource = Get-Content -LiteralPath (Join-Path $repositoryRoot "install.ps1") -Raw
    $expectedIdentity = '^https://github\.com/yigityargili991/crantcli/\.github/workflows/release\.yml@refs/tags/v[^/]+$'
    if (-not $installerSource.Contains($expectedIdentity)) {
        throw "installer does not constrain signatures to the release workflow"
    }
    if (-not $installerSource.Contains('[IO.File]::Replace($stagedPath, $Destination, $backupPath, $true)')) {
        throw "installer does not replace an existing executable in one operation"
    }
    if ($installerSource.Contains('Move-Item -LiteralPath $Destination -Destination $backupPath')) {
        throw "installer temporarily removes the canonical executable before replacement"
    }

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

    Write-Checksums
    $env:CRANTCLI_TEST_COSIGN_FAIL = "1"
    $env:PROCESSOR_ARCHITECTURE = "AMD64"
    $env:CRANTCLI_INSTALL_DIR = Join-Path $testRoot "signature-failure"
    $env:CRANTCLI_VERSION = "latest"
    try {
        & (Join-Path $repositoryRoot "install.ps1")
        throw "installer accepted a binary with an invalid signature"
    }
    catch {
        if ($_.Exception.Message -notlike "*cosign signature verification failed*") {
            throw
        }
    }
    if (Test-Path -LiteralPath (Join-Path $env:CRANTCLI_INSTALL_DIR "crantcli.exe")) {
        throw "installer copied a binary after signature verification failed"
    }

    $env:CRANTCLI_TEST_COSIGN_FAIL = $null
    $missingBundle = Join-Path $fixtures "crant_type_look-windows-amd64.exe.sigstore.json"
    $savedBundle = Join-Path $testRoot "crant_type_look-windows-amd64.exe.sigstore.json"
    Move-Item -LiteralPath $missingBundle -Destination $savedBundle
    $env:PROCESSOR_ARCHITECTURE = "AMD64"
    $env:CRANTCLI_INSTALL_DIR = Join-Path $testRoot "missing-bundle-failure"
    $env:CRANTCLI_REQUIRE_SIGNATURE = "1"
    try {
        & (Join-Path $repositoryRoot "install.ps1")
        throw "update mode accepted a binary without a signature bundle"
    }
    catch {
        if ($_.Exception.Message -notlike "*refusing an unauthenticated update*") {
            throw
        }
    }
    finally {
        Move-Item -LiteralPath $savedBundle -Destination $missingBundle
    }
    if (Test-Path -LiteralPath (Join-Path $env:CRANTCLI_INSTALL_DIR "crantcli.exe")) {
        throw "installer copied a binary without its required signature bundle"
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
    $env:CRANTCLI_REQUIRE_SIGNATURE = $originalRequireSignature
    $env:CRANTCLI_GITHUB_TOKEN = $originalGithubToken
    $env:CRANTCLI_TEST_COSIGN_FAIL = $originalCosignFail
    Remove-Item function:global:Invoke-WebRequest -ErrorAction SilentlyContinue
    Remove-Variable CrantCliInstallerTestFixtures -Scope Global -ErrorAction SilentlyContinue
    Remove-Variable CrantCliInstallerRequestedUris -Scope Global -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $testRoot) {
        Remove-Item -LiteralPath $testRoot -Recurse -Force
    }
}

# Expected negative-path checks leave the native cosign shim's exit code at 1.
# A successful test script must reset the process result for CI runners.
exit 0
