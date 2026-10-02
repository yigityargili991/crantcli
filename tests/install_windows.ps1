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
$originalVerifyBinary = $env:CRANTCLI_VERIFY_BINARY
$originalUpdatePid = $env:CRANTCLI_UPDATE_PID
$originalGithubToken = $env:CRANTCLI_GITHUB_TOKEN
$originalCosignFail = $env:CRANTCLI_TEST_COSIGN_FAIL
$originalVerifierFail = $env:CRANTCLI_TEST_VERIFIER_FAIL
$env:CRANTCLI_GITHUB_TOKEN = $null
$env:CRANTCLI_REQUIRE_SIGNATURE = $null
$env:CRANTCLI_VERIFY_BINARY = $null
$env:CRANTCLI_UPDATE_PID = $null
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
        [Parameter(Mandatory = $true)][string]$Version,
        [string]$InstallSuffix = "",
        [switch]$FreshInstall
    )

    $assetArchitecture = $Architecture.ToLowerInvariant()
    $asset = "crant_type_look-windows-$assetArchitecture.exe"
    $installDirectory = Join-Path $testRoot "install-$assetArchitecture$InstallSuffix"
    $installedFile = Join-Path $installDirectory "crantcli.exe"
    New-Item -ItemType Directory -Path $installDirectory | Out-Null
    if (-not $FreshInstall) {
        Set-Content -LiteralPath $installedFile -Value "old fixture" -NoNewline
    }
    $env:PROCESSOR_ARCHITECTURE = $Architecture
    $env:PROCESSOR_ARCHITEW6432 = $null
    $env:CRANTCLI_INSTALL_DIR = $installDirectory
    $env:CRANTCLI_VERSION = $Version
    $global:CrantCliInstallerRequestedUris.Clear()

    # Write-Host records go to the information stream (6).
    $messages = @(& (Join-Path $repositoryRoot "install.ps1") 6>&1 | ForEach-Object { [string]$_ })

    Assert-Equal `
        (Get-Content -LiteralPath (Join-Path $fixtures "crant_type_look-windows-$assetArchitecture.exe") -Raw) `
        (Get-Content -LiteralPath $installedFile -Raw) `
        "$Architecture asset was not installed"
    $suggestedSetup = $messages -contains "Next: crantcli setup"
    if ($FreshInstall -and -not $suggestedSetup) {
        throw "fresh install did not suggest crantcli setup"
    }
    if (-not $FreshInstall -and $suggestedSetup) {
        throw "installer suggested setup when replacing an existing binary"
    }
    if (Get-ChildItem -LiteralPath $installDirectory -Filter "crantcli.exe.old*" -File) {
        throw "installer left an unlocked backup in $installDirectory"
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
    $bundleSuffix = if ([string]::IsNullOrWhiteSpace($env:CRANTCLI_VERIFY_BINARY)) {
        ".sigstore.json"
    }
    else {
        ".bundle.json"
    }
    if (-not ($global:CrantCliInstallerRequestedUris | Where-Object { $_.EndsWith("$asset$bundleSuffix") })) {
        throw "installer did not download the signature bundle for $asset"
    }
}

function Test-LockedBinaryCleanup {
    param(
        [Parameter(Mandatory = $true)][string]$Scenario,
        [string]$BackupSuffix = "",
        [switch]$LegacyUpdater,
        [switch]$CleanupLaunchFails
    )

    # Use a running executable to reproduce the Windows image lock; text
    # fixtures alone cannot exercise cleanup after crantcli update exits.
    $installDirectory = Join-Path $testRoot ("$Scenario [test] 'unicode-" + [char]0x03bb)
    $installedFile = Join-Path $installDirectory "crantcli.exe"
    $ready = Join-Path $installDirectory "ready"
    $stop = Join-Path $installDirectory "stop"
    New-Item -ItemType Directory -Path $installDirectory | Out-Null
    Copy-Item -LiteralPath (Join-Path $testRoot "lock-fixture.exe") -Destination $installedFile
    $process = Start-Process -FilePath $installedFile -WindowStyle Hidden -PassThru `
        -ArgumentList "`"$ready`" `"$stop`""
    try {
        $deadline = [DateTime]::UtcNow.AddSeconds(15)
        while (-not (Test-Path -LiteralPath $ready)) {
            if ($process.HasExited -or [DateTime]::UtcNow -ge $deadline) {
                throw "locked executable fixture did not start"
            }
            Start-Sleep -Milliseconds 50
        }

        if ($BackupSuffix -ne "") {
            Move-Item -LiteralPath $installedFile -Destination "${installedFile}$BackupSuffix"
            Set-Content -LiteralPath $installedFile -Value "old fixture" -NoNewline
        }
        $unrelated = Join-Path $installDirectory "crantcli.exe.old-not-an-installer-backup"
        Set-Content -LiteralPath $unrelated -Value "keep me" -NoNewline
        $env:PROCESSOR_ARCHITECTURE = "AMD64"
        $env:PROCESSOR_ARCHITEW6432 = $null
        $env:CRANTCLI_INSTALL_DIR = $installDirectory
        $env:CRANTCLI_VERSION = "latest"
        $env:CRANTCLI_UPDATE_PID = if ($LegacyUpdater) { $null } else { [string]$process.Id }
        if ($CleanupLaunchFails) {
            function global:Start-Process {
                throw "cleanup launch failed"
            }
        }
        try {
            & (Join-Path $repositoryRoot "install.ps1")
        }
        finally {
            if ($CleanupLaunchFails) {
                Remove-Item function:global:Start-Process
            }
        }

        Assert-Equal `
            (Get-Content -LiteralPath (Join-Path $fixtures "crant_type_look-windows-amd64.exe") -Raw) `
            (Get-Content -LiteralPath $installedFile -Raw) `
            "installer did not replace the binary while its previous copy was running"
        $backups = @(Get-ChildItem -LiteralPath $installDirectory -Filter "crantcli.exe.old*" -File |
            Where-Object { $_.Name -ne "crantcli.exe.old-not-an-installer-backup" })
        Assert-Equal 1 $backups.Count "installer did not retain exactly the locked backup"
        if ($process.HasExited) {
            throw "installer terminated the running executable"
        }
        Assert-Equal "keep me" (Get-Content -LiteralPath $unrelated -Raw) "cleanup removed an unrelated file"

        Set-Content -LiteralPath $stop -Value "stop"
        if (-not $process.WaitForExit(15000)) {
            throw "locked executable fixture did not exit"
        }
        if (-not $CleanupLaunchFails) {
            $deadline = [DateTime]::UtcNow.AddSeconds(15)
            while (Test-Path -LiteralPath $backups[0].FullName) {
                if ([DateTime]::UtcNow -ge $deadline) {
                    throw "backup was not removed after the updating process exited: $($backups[0].FullName)"
                }
                Start-Sleep -Milliseconds 50
            }
        }
    }
    finally {
        $env:CRANTCLI_UPDATE_PID = $null
        if (-not $process.HasExited) {
            Stop-Process -Id $process.Id -Force
            [void]$process.WaitForExit(15000)
        }
        $process.Dispose()
    }
}

New-Item -ItemType Directory -Path $fixtures, $fakeBin | Out-Null
try {
    foreach ($architecture in @("amd64", "arm64")) {
        $asset = "crant_type_look-windows-$architecture.exe"
        Set-Content -LiteralPath (Join-Path $fixtures $asset) -Value "$architecture fixture" -NoNewline
        Set-Content -LiteralPath (Join-Path $fixtures "$asset.sigstore.json") -Value '{"fixture":"signature"}' -NoNewline
        Set-Content -LiteralPath (Join-Path $fixtures "$asset.bundle.json") -Value '{"fixture":"signature"}' -NoNewline
    }
    Set-Content -LiteralPath (Join-Path $fakeBin "cosign.cmd") -Value @(
        '@echo off',
        'if "%CRANTCLI_TEST_COSIGN_FAIL%"=="1" exit /b 1',
        'exit /b 0'
    )
    Set-Content -LiteralPath (Join-Path $fakeBin "crantcli-verifier.cmd") -Value @(
        '@echo off',
        'if "%CRANTCLI_TEST_VERIFIER_FAIL%"=="1" exit /b 1',
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
    Test-Install -Architecture "AMD64" -Version "latest" -InstallSuffix "-fresh" -FreshInstall

    $env:CRANTCLI_VERIFY_BINARY = Join-Path $fakeBin "crantcli-verifier.cmd"
    Test-Install -Architecture "AMD64" -Version "latest" -InstallSuffix "-builtin"
    $env:CRANTCLI_VERIFY_BINARY = $null

    $fixtureSource = Join-Path $testRoot "lock-fixture.go"
    Set-Content -LiteralPath $fixtureSource -Encoding UTF8 -Value @'
package main

import (
    "os"
    "time"
)

func main() {
    if err := os.WriteFile(os.Args[1], []byte("ready"), 0600); err != nil {
        os.Exit(1)
    }
    deadline := time.Now().Add(time.Minute)
    for time.Now().Before(deadline) {
        if _, err := os.Stat(os.Args[2]); err == nil {
            return
        }
        time.Sleep(20 * time.Millisecond)
    }
}
'@
    & go build -o (Join-Path $testRoot "lock-fixture.exe") $fixtureSource
    if ($LASTEXITCODE -ne 0) {
        throw "could not build the executable lock fixture"
    }
    Test-LockedBinaryCleanup -Scenario "running-binary"
    Test-LockedBinaryCleanup -Scenario "legacy-backup" -BackupSuffix ".old"
    Test-LockedBinaryCleanup -Scenario "previous-backup" -BackupSuffix ".old-$([Guid]::NewGuid().ToString('N'))"
    Test-LockedBinaryCleanup -Scenario "legacy-updater" -LegacyUpdater
    Test-LockedBinaryCleanup -Scenario "cleanup-failure" -CleanupLaunchFails

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
    $env:CRANTCLI_TEST_VERIFIER_FAIL = "1"
    $env:CRANTCLI_VERIFY_BINARY = Join-Path $fakeBin "crantcli-verifier.cmd"
    $env:CRANTCLI_INSTALL_DIR = Join-Path $testRoot "builtin-signature-failure"
    try {
        & (Join-Path $repositoryRoot "install.ps1")
        throw "installer accepted a signature rejected by the built-in verifier"
    }
    catch {
        if ($_.Exception.Message -notlike "*Sigstore signature verification failed*") {
            throw
        }
    }
    if (Test-Path -LiteralPath (Join-Path $env:CRANTCLI_INSTALL_DIR "crantcli.exe")) {
        throw "installer copied a binary after built-in signature verification failed"
    }

    $env:CRANTCLI_TEST_VERIFIER_FAIL = $null
    $env:CRANTCLI_VERIFY_BINARY = $null
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
    $env:CRANTCLI_VERIFY_BINARY = $originalVerifyBinary
    $env:CRANTCLI_UPDATE_PID = $originalUpdatePid
    $env:CRANTCLI_GITHUB_TOKEN = $originalGithubToken
    $env:CRANTCLI_TEST_COSIGN_FAIL = $originalCosignFail
    $env:CRANTCLI_TEST_VERIFIER_FAIL = $originalVerifierFail
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
