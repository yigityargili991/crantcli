& {

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
Set-StrictMode -Version 2.0

$RepoUrl = "https://github.com/yigityargili991/crantcli"
$Repository = "yigityargili991/crantcli"
$AssetPrefix = "crant_type_look"
$BinaryName = "crantcli.exe"
$OriginalCrantCliGithubToken = [Environment]::GetEnvironmentVariable("CRANTCLI_GITHUB_TOKEN", "Process")
$GithubToken = $OriginalCrantCliGithubToken
[Environment]::SetEnvironmentVariable("CRANTCLI_GITHUB_TOKEN", $null, "Process")

function Write-InstallerMessage {
    param([Parameter(Mandatory = $true)][string]$Message)

    Write-Host $Message
}

function Invoke-InstallerDownload {
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string]$OutFile,
        [Parameter(Mandatory = $true)][string]$Version
    )

    if (-not [string]::IsNullOrWhiteSpace($GithubToken)) {
        $gh = Get-Command gh -ErrorAction SilentlyContinue
        if ($null -eq $gh) {
            throw "gh is required when CRANTCLI_GITHUB_TOKEN is set"
        }

        $arguments = [Collections.Generic.List[string]]::new()
        $arguments.Add("release")
        $arguments.Add("download")
        if ($Version -ne "latest") {
            $arguments.Add($Version)
        }
        $arguments.Add("--repo")
        $arguments.Add($Repository)
        $arguments.Add("--pattern")
        $arguments.Add([IO.Path]::GetFileName(([Uri]$Uri).AbsolutePath))
        $arguments.Add("--output")
        $arguments.Add($OutFile)

        $originalGhToken = [Environment]::GetEnvironmentVariable("GH_TOKEN", "Process")
        try {
            $env:GH_TOKEN = $GithubToken
            & $gh.Source @arguments | Out-Null
            $exitCode = $LASTEXITCODE
        }
        finally {
            [Environment]::SetEnvironmentVariable("GH_TOKEN", $originalGhToken, "Process")
        }
        if ($exitCode -ne 0) {
            throw "GitHub CLI could not download $Uri"
        }
        return
    }

    $parameters = @{
        Uri         = $Uri
        OutFile     = $OutFile
        ErrorAction = "Stop"
    }
    if ($PSVersionTable.PSVersion.Major -lt 6) {
        $parameters["UseBasicParsing"] = $true
    }
    Invoke-WebRequest @parameters
}

function Test-InstallerDownload {
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string]$OutFile,
        [Parameter(Mandatory = $true)][string]$Version
    )

    try {
        Invoke-InstallerDownload -Uri $Uri -OutFile $OutFile -Version $Version
        return $true
    }
    catch {
        return $false
    }
}

function Get-WindowsArchitecture {
    $architecture = $env:PROCESSOR_ARCHITEW6432
    if ([string]::IsNullOrWhiteSpace($architecture)) {
        $architecture = $env:PROCESSOR_ARCHITECTURE
    }
    if ([string]::IsNullOrWhiteSpace($architecture)) {
        throw "could not determine the Windows architecture"
    }

    switch ($architecture.ToUpperInvariant()) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { throw "unsupported Windows architecture: $architecture" }
    }
}

function Test-PathContains {
    param(
        [AllowNull()][string]$PathValue,
        [Parameter(Mandatory = $true)][string]$Directory
    )

    $target = [IO.Path]::GetFullPath($Directory).TrimEnd("\")
    foreach ($entry in ($PathValue -split ";")) {
        if ([string]::IsNullOrWhiteSpace($entry)) {
            continue
        }

        $expandedEntry = [Environment]::ExpandEnvironmentVariables($entry.Trim().Trim('"'))
        try {
            $candidate = [IO.Path]::GetFullPath($expandedEntry).TrimEnd("\")
        }
        catch {
            continue
        }

        if ([StringComparer]::OrdinalIgnoreCase.Equals($candidate, $target)) {
            return $true
        }
    }

    return $false
}

function Add-InstallDirectoryToPath {
    param([Parameter(Mandatory = $true)][string]$InstallDirectory)

    if (-not (Test-PathContains -PathValue $env:Path -Directory $InstallDirectory)) {
        $env:Path = "$InstallDirectory;$env:Path"
    }

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (-not (Test-PathContains -PathValue $userPath -Directory $InstallDirectory)) {
        $newUserPath = if ([string]::IsNullOrWhiteSpace($userPath)) {
            $InstallDirectory
        }
        else {
            "$userPath;$InstallDirectory"
        }
        [Environment]::SetEnvironmentVariable("Path", $newUserPath, "User")
        Write-InstallerMessage "Added $InstallDirectory to your user PATH"
    }
}

function Install-CrantCli {
    if ([Environment]::OSVersion.Platform -ne [PlatformID]::Win32NT) {
        throw "install.ps1 only supports Windows"
    }

    if ([Net.ServicePointManager]::SecurityProtocol -band [Net.SecurityProtocolType]::Tls12) {
        # TLS 1.2 is already enabled.
    }
    else {
        [Net.ServicePointManager]::SecurityProtocol =
            [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    }

    $architecture = Get-WindowsArchitecture
    $version = if ([string]::IsNullOrWhiteSpace($env:CRANTCLI_VERSION)) {
        "latest"
    }
    else {
        $env:CRANTCLI_VERSION.Trim()
    }

    $installDirectory = if ([string]::IsNullOrWhiteSpace($env:CRANTCLI_INSTALL_DIR)) {
        if ([string]::IsNullOrWhiteSpace($env:LOCALAPPDATA)) {
            throw "LOCALAPPDATA is not set; set CRANTCLI_INSTALL_DIR to choose an install directory"
        }
        Join-Path $env:LOCALAPPDATA "Programs\crantcli"
    }
    else {
        [Environment]::ExpandEnvironmentVariables($env:CRANTCLI_INSTALL_DIR)
    }
    $installDirectory = [IO.Path]::GetFullPath($installDirectory)

    $releaseBase = if ($version -eq "latest") {
        "$RepoUrl/releases/latest/download"
    }
    else {
        "$RepoUrl/releases/download/$version"
    }

    $asset = "$AssetPrefix-windows-$architecture.exe"
    $temporaryDirectory = Join-Path ([IO.Path]::GetTempPath()) "crantcli-$([Guid]::NewGuid().ToString("N"))"
    $downloadedBinary = Join-Path $temporaryDirectory $asset
    $checksumsFile = Join-Path $temporaryDirectory "checksums.txt"

    New-Item -ItemType Directory -Path $temporaryDirectory | Out-Null
    try {
        Write-InstallerMessage "Installing crantcli $version for windows/$architecture"
        Invoke-InstallerDownload -Uri "$releaseBase/$asset" -OutFile $downloadedBinary -Version $version

        if ($env:CRANTCLI_SKIP_CHECKSUM -eq "1") {
            Write-Warning "CRANTCLI_SKIP_CHECKSUM=1 set; skipping checksum verification (insecure)"
        }
        else {
            try {
                Invoke-InstallerDownload -Uri "$releaseBase/checksums.txt" -OutFile $checksumsFile -Version $version
            }
            catch {
                throw "could not download checksums.txt for $version; refusing to install an unverified binary (set CRANTCLI_SKIP_CHECKSUM=1 to override)"
            }

            $expectedHash = $null
            foreach ($line in (Get-Content -LiteralPath $checksumsFile)) {
                if ($line -match "^([0-9a-fA-F]{64})\s+\*?(.+?)\s*$" -and $Matches[2] -eq $asset) {
                    $expectedHash = $Matches[1]
                    break
                }
            }
            if ([string]::IsNullOrWhiteSpace($expectedHash)) {
                throw "checksums.txt does not contain an entry for $asset"
            }

            $actualHash = (Get-FileHash -LiteralPath $downloadedBinary -Algorithm SHA256).Hash
            if (-not [StringComparer]::OrdinalIgnoreCase.Equals($actualHash, $expectedHash)) {
                throw "checksum mismatch for $asset"
            }
            Write-InstallerMessage "Verified checksum for $asset"
        }

        $cosign = Get-Command cosign -ErrorAction SilentlyContinue
        if ($null -ne $cosign) {
            $bundle = Join-Path $temporaryDirectory "$asset.sigstore.json"
            if (Test-InstallerDownload -Uri "$releaseBase/$asset.sigstore.json" -OutFile $bundle -Version $version) {
                & $cosign.Source verify-blob --bundle $bundle `
                    --certificate-oidc-issuer "https://token.actions.githubusercontent.com" `
                    --certificate-identity-regexp "^https://github\.com/yigityargili991/crantcli/\.github/workflows/release\.yml@refs/tags/v[^/]+$" `
                    $downloadedBinary | Out-Null
                if ($LASTEXITCODE -ne 0) {
                    throw "cosign signature verification failed for $asset"
                }
                Write-InstallerMessage "Verified cosign signature for $asset"
            }
            else {
                Write-Warning "no cosign signature bundle found for $version; relying on checksum verification"
            }
        }
        else {
            Write-Warning "cosign not installed; relying on checksum verification (install cosign for signature verification)"
        }

        New-Item -ItemType Directory -Path $installDirectory -Force | Out-Null
        $installPath = Join-Path $installDirectory $BinaryName
        Copy-Item -LiteralPath $downloadedBinary -Destination $installPath -Force
        Add-InstallDirectoryToPath -InstallDirectory $installDirectory

        Write-InstallerMessage "Installed crantcli to $installPath"
        Write-InstallerMessage "Next: crantcli setup"
    }
    finally {
        if (Test-Path -LiteralPath $temporaryDirectory) {
            Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force
        }
    }
}

try {
    Install-CrantCli
}
finally {
    [Environment]::SetEnvironmentVariable("CRANTCLI_GITHUB_TOKEN", $OriginalCrantCliGithubToken, "Process")
}
}
