# utuimp installer for Windows — https://github.com/tphx37/utuimp
#
#   irm https://raw.githubusercontent.com/tphx37/utuimp/main/scripts/install.ps1 | iex
#
# It looks for what utuimp needs, offers to install what is missing with
# winget, builds utuimp from source and puts utuimp.exe in
# %LOCALAPPDATA%\Programs\utuimp (added to your PATH).
# Set $env:UTUIMP_YES = "1" to install without questions.

$ErrorActionPreference = "Stop"
$repo = "tphx37/utuimp"
$dir = if ($env:UTUIMP_INSTALL_DIR) { $env:UTUIMP_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\utuimp" }

function Have($cmd) { [bool](Get-Command $cmd -ErrorAction SilentlyContinue) }
function Step($t) { Write-Host "`n==> $t" -ForegroundColor Cyan }
function Ask($q) {
    if ($env:UTUIMP_YES -eq "1") { return $true }
    $a = Read-Host "    $q [Y/n]"
    return -not ($a -match '^[nN]')
}

# name, command, winget id, needed to build, what for, where to get it by hand
$deps = @(
    @{ n = "Go";     c = "go";     id = "GoLang.Go";     need = $true;  why = "builds utuimp";                url = "https://go.dev/doc/install" },
    @{ n = "Git";    c = "git";    id = "Git.Git";       need = $true;  why = "fetches the source";           url = "https://git-scm.com/downloads/win" },
    @{ n = "yt-dlp"; c = "yt-dlp"; id = "yt-dlp.yt-dlp"; need = $false; why = "YouTube Music and SoundCloud"; url = "https://github.com/yt-dlp/yt-dlp/wiki/Installation" },
    @{ n = "FFmpeg"; c = "ffmpeg"; id = "Gyan.FFmpeg";   need = $false; why = "YouTube Music and SoundCloud"; url = "https://ffmpeg.org/download.html" }
)

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

Step "Building utuimp from source..."
$tmp = Join-Path ([IO.Path]::GetTempPath()) ("utuimp-" + [Guid]::NewGuid().ToString("N"))
try {
    git clone --depth 1 "https://github.com/$repo" $tmp | Out-Null
    Push-Location $tmp
    try { go build -o utuimp.exe . ; if ($LASTEXITCODE -ne 0) { throw "The build failed." } }
    finally { Pop-Location }
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Copy-Item (Join-Path $tmp "utuimp.exe") (Join-Path $dir "utuimp.exe") -Force
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
Write-Host "    ok  $dir\utuimp.exe"

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($userPath -split ";") -notcontains $dir) {
    [Environment]::SetEnvironmentVariable("Path", ($userPath.TrimEnd(";") + ";" + $dir), "User")
    Write-Host "    Added $dir to your PATH. Open a new terminal."
}
try { & (Join-Path $dir "utuimp.exe") setup-desktop | Out-Null } catch {}

Write-Host "`n    utuimp is installed. Run: utuimp`n" -ForegroundColor Green
