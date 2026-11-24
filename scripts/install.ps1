# utuimp installer for Windows — https://github.com/tphx37/utuimp
#
#   irm https://raw.githubusercontent.com/tphx37/utuimp/main/scripts/install.ps1 | iex
#
# It looks for what utuimp needs, offers to install what is missing with
# winget, downloads a verified release (or builds from source) and puts utuimp.exe in
# %LOCALAPPDATA%\Programs\utuimp (added to your PATH).
# Set $env:UTUIMP_YES = "1" to install without questions.

$ErrorActionPreference = "Stop"
$repo = "tphx37/utuimp"
$existing = Get-Command utuimp.exe -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
$dir = if ($env:UTUIMP_INSTALL_DIR) { $env:UTUIMP_INSTALL_DIR }
       elseif ($existing) { Split-Path -Parent $existing.Source }
       else { Join-Path $env:LOCALAPPDATA "Programs\utuimp" }
$dir = [IO.Path]::GetFullPath($dir)

function Have($cmd) { [bool](Get-Command $cmd -ErrorAction SilentlyContinue) }
function Step($t) { Write-Host "`n==> $t" -ForegroundColor Cyan }
function Ask($q) {
    if ($env:UTUIMP_YES -eq "1") { return $true }
    $a = Read-Host "    $q [Y/n]"
    return -not ($a -match '^[nN]')
}

# name, command, winget id, needed to build, what for, where to get it by hand
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("utuimp-" + [Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
$release = $false
$archive = Join-Path $tmp "utuimp_windows_amd64.zip"
try {
    $arch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    if ($arch -eq "AMD64") {
        Step "Looking for the latest release..."
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Uri "https://github.com/$repo/releases/latest"
            $uri = if ($response.BaseResponse.ResponseUri) { $response.BaseResponse.ResponseUri.AbsoluteUri } else { $response.BaseResponse.RequestMessage.RequestUri.AbsoluteUri }
            if ($uri -notmatch '/releases/tag/([^/]+)$') { throw "Could not determine the latest release tag." }
            $tag = $Matches[1]
            $base = "https://github.com/$repo/releases/download/$tag"
            Invoke-WebRequest -UseBasicParsing -Uri "$base/utuimp_windows_amd64.zip" -OutFile $archive
            Invoke-WebRequest -UseBasicParsing -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp "checksums.txt")
            $release = $true
        } catch {
            Write-Host "    Release download unavailable; will build from source. $($_.Exception.Message)" -ForegroundColor Yellow
        }
    }
    if ($release) {
        $line = Get-Content (Join-Path $tmp "checksums.txt") | Where-Object { $_ -match '^([a-fA-F0-9]{64})\s+\*?utuimp_windows_amd64\.zip$' } | Select-Object -First 1
        if (-not $line -or $line -notmatch '^([a-fA-F0-9]{64})') { throw "Release checksum is missing; refusing to install." }
        $expected = $Matches[1]
        if ((Get-FileHash -Algorithm SHA256 $archive).Hash -ne $expected) { throw "Release checksum mismatch; refusing to install." }
        Write-Host "    Release checksum OK"
    }

$deps = @(
    @{ n = "Go";     c = "go";     id = "GoLang.Go";     need = $true;  why = "builds utuimp";                url = "https://go.dev/doc/install" },
    @{ n = "Git";    c = "git";    id = "Git.Git";       need = $true;  why = "fetches the source";           url = "https://git-scm.com/downloads/win" },
    @{ n = "yt-dlp"; c = "yt-dlp"; id = "yt-dlp.yt-dlp"; need = $false; why = "YouTube Music and SoundCloud"; url = "https://github.com/yt-dlp/yt-dlp/wiki/Installation" },
    @{ n = "FFmpeg"; c = "ffmpeg"; id = "Gyan.FFmpeg";   need = $false; why = "YouTube Music and SoundCloud"; url = "https://ffmpeg.org/download.html" }
)
if ($release) { $deps = @($deps | Where-Object { -not $_.need }) }

Write-Host "`n    utuimp installer" -ForegroundColor Green
Step "Checking what utuimp needs..."
$missing = @()
foreach ($d in $deps) {
    if (Have $d.c) { Write-Host "    ok  $($d.n)" }
    else {
        $kind = if ($d.need) { "missing" } else { "not installed (optional)" }
        Write-Host "    --  $($d.n) is $kind`: $($d.why)" -ForegroundColor Yellow
        $missing += $d
    }
}
if (-not (Have "librespot")) {
    Write-Host "    --  librespot is not installed (optional: Spotify playback): https://github.com/librespot-org/librespot/wiki/Compiling" -ForegroundColor Yellow
}

if ($missing.Count -gt 0) {
    if (Have "winget") {
        $ids = ($missing | ForEach-Object { $_.id }) -join " "
        Write-Host "    winget install $ids"
        if (Ask "Install them now?") {
            foreach ($d in $missing) {
                winget install --id $d.id -e --accept-source-agreements --accept-package-agreements
            }
            # What winget just put on PATH, for this session too.
            $env:Path = [Environment]::GetEnvironmentVariable("Path", "Machine") + ";" + [Environment]::GetEnvironmentVariable("Path", "User")
        }
    } else {
        Write-Host "    winget not found. Install by hand:" -ForegroundColor Yellow
        foreach ($d in $missing) { Write-Host "      $($d.n): $($d.url)" }
    }
}
foreach ($d in $deps) {
    if ($d.need -and -not (Have $d.c)) { throw "$($d.n) is still missing: $($d.url)" }
}

    if ($release) {
        Step "Installing release $tag..."
        $source = Join-Path $tmp "release"
        Expand-Archive -Path $archive -DestinationPath $source
    } else {
        Step "Building utuimp from source..."
        $source = Join-Path $tmp "src"
        git clone --depth 1 "https://github.com/$repo" $source | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "Could not clone the source." }
        Push-Location $source
        try { go build -o utuimp.exe . ; if ($LASTEXITCODE -ne 0) { throw "The build failed." } }
        finally { Pop-Location }
    }
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Copy-Item (Join-Path $source "utuimp.exe") (Join-Path $dir "utuimp.exe") -Force
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
Write-Host "    ok  $dir\utuimp.exe"
# Make the installed version available in this PowerShell session as well.
$env:Path = $dir + ";" + $env:Path

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ";") -notcontains $dir) {
    [Environment]::SetEnvironmentVariable("Path", ($userPath.TrimEnd(";") + ";" + $dir), "User")
    Write-Host "    Added $dir to your PATH. Open a new terminal."
}
try { & (Join-Path $dir "utuimp.exe") setup-desktop | Out-Null } catch {}

$resolved = Get-Command utuimp -ErrorAction SilentlyContinue
if ($resolved -and $resolved.CommandType -ne 'Application') {
    Write-Warning "The command 'utuimp' resolves to $($resolved.CommandType) $($resolved.Name), which takes precedence over the installed executable. Remove that alias/function or run '$dir\utuimp.exe'."
}
Write-Host "`n    utuimp is installed. Run: utuimp`n" -ForegroundColor Green
