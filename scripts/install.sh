#!/usr/bin/env sh
# utuimp installer for Linux and macOS — https://github.com/tphx37/utuimp
# (Windows: scripts/install.ps1)
#
#   curl -fsSL https://raw.githubusercontent.com/tphx37/utuimp/main/scripts/install.sh | sh
#
# What it does:
#   1. looks for what utuimp needs and offers to install what is missing
#      with your package manager (pacman, apt, dnf, zypper, brew);
#   2. installs utuimp to ~/.local/bin: a published release when there is
#      one for your system, otherwise built from source.
#
# Options (also after `sh -s --` when piped):
#   --deps     only check and offer to install dependencies (make uses this)
#   --source   build from source even when a release exists
#   --yes      don't ask, install what is missing
# Environment: UTUIMP_INSTALL_DIR (default ~/.local/bin), UTUIMP_YES=1.

set -eu

REPO="tphx37/utuimp"
BIN="utuimp"
INSTALL_DIR="${UTUIMP_INSTALL_DIR:-${HOME}/.local/bin}"
DEPS_ONLY=0
FROM_SOURCE=0
YES="${UTUIMP_YES:-0}"
for a in "$@"; do
    case "$a" in
        --deps) DEPS_ONLY=1 ;;
        --source) FROM_SOURCE=1 ;;
        --yes|-y) YES=1 ;;
        *) printf 'unknown option: %s\n' "$a" >&2; exit 2 ;;
    esac
done

# ── output ────────────────────────────────────────────────────────────────────

if [ -t 1 ] && command -v tput >/dev/null 2>&1; then
    BOLD="$(tput bold 2>/dev/null || printf '')"; DIM="$(tput dim 2>/dev/null || printf '')"
    MINT="$(tput setaf 2 2>/dev/null || printf '')"; CYAN="$(tput setaf 6 2>/dev/null || printf '')"
    YELLOW="$(tput setaf 3 2>/dev/null || printf '')"; RED="$(tput setaf 1 2>/dev/null || printf '')"
    RESET="$(tput sgr0 2>/dev/null || printf '')"
else
    BOLD="" DIM="" MINT="" CYAN="" YELLOW="" RED="" RESET=""
fi

step()    { printf '\n%s==>%s %s%s%s\n' "${BOLD}${CYAN}" "${RESET}" "${BOLD}" "$*" "${RESET}"; }
info()    { printf '    %s%s%s\n' "${DIM}" "$*" "${RESET}"; }
success() { printf '    %s✓%s  %s\n' "${MINT}" "${RESET}" "$*"; }
warn()    { printf '    %s!%s  %s\n' "${YELLOW}" "${RESET}" "$*" >&2; }
die()     { printf '\n    %s✗%s  %s\n\n' "${RED}" "${RESET}" "$*" >&2; exit 1; }
have()    { command -v "$1" >/dev/null 2>&1; }

# ask "question": yes unless answered n. Piped from curl, stdin is the
# script itself, so the answer is read from the terminal; with no terminal
# (CI) nothing is installed unless --yes was given.
ask() {
    [ "${YES}" = "1" ] && return 0
    if ! { [ -r /dev/tty ] && [ -w /dev/tty ] && (: </dev/tty) 2>/dev/null; }; then
        return 1
    fi
    printf '    %s [Y/n] ' "$1" >/dev/tty
    read -r reply </dev/tty || return 1
    case "${reply}" in n*|N*) return 1 ;; *) return 0 ;; esac
}

download() { # url file
    if have curl; then curl --proto '=https' --tlsv1.2 -sSfL -o "$2" "$1"
    elif have wget; then wget -qO "$2" "$1"
    else die "Neither curl nor wget found: install one and try again."; fi
}
fetch() { # url → stdout
    if have curl; then curl --proto '=https' --tlsv1.2 -sSfL "$1"
    elif have wget; then wget -qO- "$1"
    else return 1; fi
}

# ── platform ──────────────────────────────────────────────────────────────────

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)             ARCH="$(uname -m)" ;;
esac
case "${OS}" in
    linux|darwin) ;;
    *) die "This script is for Linux and macOS (got ${OS}). On Windows use scripts/install.ps1." ;;
esac

PM=""
for p in pacman apt-get dnf zypper brew; do
    if have "$p"; then PM="$p"; break; fi
done
SUDO=""
if [ "${PM}" != "brew" ] && [ "$(id -u)" != "0" ]; then SUDO="sudo"; fi

# pkgs <what>: the packages that provide it with this package manager
# (nothing when it isn't packaged there).
pkgs() {
    case "${PM}:$1" in
        pacman:go) echo go ;;            apt-get:go) echo golang-go ;;   dnf:go) echo golang ;;   zypper:go) echo go ;;   brew:go) echo go ;;
        pacman:git|apt-get:git|dnf:git|zypper:git|brew:git) echo git ;;
        pacman:cc) echo gcc pkgconf ;;   apt-get:cc) echo build-essential pkg-config ;;
        dnf:cc) echo gcc pkgconf-pkg-config ;;   zypper:cc) echo gcc pkg-config ;;
        pacman:gstreamer) echo gstreamer gst-plugins-base ;;
        apt-get:gstreamer) echo libgstreamer1.0-dev libgstreamer-plugins-base1.0-dev gstreamer1.0-plugins-base ;;
        dnf:gstreamer) echo gstreamer1-devel gstreamer1-plugins-base-devel gstreamer1-plugins-base ;;
        zypper:gstreamer) echo gstreamer-devel gstreamer-plugins-base-devel gstreamer-plugins-base ;;
        pacman:gst-good) echo gst-plugins-good ;;       apt-get:gst-good) echo gstreamer1.0-plugins-good ;;
        dnf:gst-good) echo gstreamer1-plugins-good ;;   zypper:gst-good) echo gstreamer-plugins-good ;;
        pacman:gst-libav) echo gst-libav ;;             apt-get:gst-libav) echo gstreamer1.0-libav ;;
        dnf:gst-libav) echo gstreamer1-plugin-libav ;;  zypper:gst-libav) echo gstreamer-plugins-libav ;;
        pacman:yt-dlp|apt-get:yt-dlp|dnf:yt-dlp|zypper:yt-dlp|brew:yt-dlp) echo yt-dlp ;;
        pacman:ffmpeg|apt-get:ffmpeg|zypper:ffmpeg|brew:ffmpeg) echo ffmpeg ;;   dnf:ffmpeg) echo ffmpeg-free ;;
        pacman:librespot|brew:librespot) echo librespot ;;
    esac
}

# where <what>: the page to install it from by hand.
where() {
    case "$1" in
        go)        echo "https://go.dev/doc/install" ;;
        git)       echo "https://git-scm.com/downloads" ;;
        cc)        echo "https://gcc.gnu.org/install/binaries.html" ;;
        gstreamer|gst-good|gst-libav) echo "https://gstreamer.freedesktop.org/documentation/installing/on-linux.html" ;;
        yt-dlp)    echo "https://github.com/yt-dlp/yt-dlp/wiki/Installation" ;;
        ffmpeg)    echo "https://ffmpeg.org/download.html" ;;
        librespot) echo "https://github.com/librespot-org/librespot/wiki/Compiling" ;;
    esac
}

# missing <what>: true when it isn't there.
missing() {
    case "$1" in
        go|git|yt-dlp|ffmpeg|librespot) ! have "$1" ;;
        cc)        ! { { have cc || have gcc || have clang; } && have pkg-config; } ;;
        gstreamer) ! { have pkg-config && pkg-config --exists gstreamer-1.0 2>/dev/null; } ;;
        # Without gst-inspect there is no telling: gstreamer itself is asked for first.
        gst-good)  have gst-inspect-1.0 && ! gst-inspect-1.0 souphttpsrc >/dev/null 2>&1 ;;
        gst-libav) have gst-inspect-1.0 && ! gst-inspect-1.0 avdec_aac >/dev/null 2>&1 ;;
    esac
}

why() {
    case "$1" in
        go) echo "builds utuimp" ;;  git) echo "fetches the source" ;;  cc) echo "C compiler and pkg-config, for the GStreamer player" ;;
        gstreamer) echo "plays the sound" ;;  gst-good) echo "FLAC, M4A, WAV and network streams" ;;  gst-libav) echo "AAC and other codecs" ;;
        yt-dlp) echo "YouTube Music and SoundCloud" ;;  ffmpeg) echo "YouTube Music and SoundCloud" ;;  librespot) echo "Spotify playback" ;;
    esac
}

# install_list "a b c": offer to install them in one go.
install_list() {
    names="$1"; packages=""; manual=""
    for n in ${names}; do
        p="$(pkgs "$n")"
        if [ -n "$p" ]; then packages="${packages} $p"; else manual="${manual} $n"; fi
    done
    if [ -n "${packages}" ]; then
        case "${PM}" in
            pacman)  cmd="${SUDO} pacman -S --needed${packages}" ;;
            apt-get) cmd="${SUDO} apt-get install -y${packages}" ;;
            dnf)     cmd="${SUDO} dnf install -y${packages}" ;;
            zypper)  cmd="${SUDO} zypper install -y${packages}" ;;
            brew)    cmd="brew install${packages}" ;;
        esac
        info "${cmd}"
        if ask "Install them now?"; then
            # The package manager asks its own questions on the terminal.
            if [ -r /dev/tty ] && (: </dev/tty) 2>/dev/null; then sh -c "${cmd}" </dev/tty || warn "The package manager failed."
            else sh -c "${cmd}" || warn "The package manager failed."; fi
        else
            info "Skipped."
        fi
    fi
    for n in ${manual}; do
        warn "$n isn't packaged for ${PM:-this system}: see $(where "$n")"
    done
}

# check_deps "required names" "optional names" → sets STILL_MISSING
check_deps() {
    need=""; opt=""
    for n in $1; do
        if missing "$n"; then need="${need} $n"; warn "$n is missing ($(why "$n"))"; else success "$n"; fi
    done
    for n in $2; do
        if missing "$n"; then opt="${opt} $n"; info "· $n is not installed (optional: $(why "$n"))"; else success "$n"; fi
    done
    if [ -n "${need}${opt}" ]; then
        if [ -z "${PM}" ]; then
            warn "No known package manager here. Install by hand:"
            for n in ${need} ${opt}; do info "$n: $(where "$n")"; done
        else
            [ -n "${need}" ] && install_list "${need}"
            [ -n "${opt}" ] && { printf '\n'; info "Optional, for the services named above:"; install_list "${opt}"; }
        fi
    fi
    STILL_MISSING=""
    for n in $1; do
        if missing "$n"; then STILL_MISSING="${STILL_MISSING} $n"; fi
    done
}

RUNTIME=""; BUILD="go"; OPTIONAL="yt-dlp ffmpeg librespot"
if [ "${OS}" = "linux" ]; then
    RUNTIME="gstreamer gst-good gst-libav"
    BUILD="go cc"
fi

# ── --deps: what `make build` asks for ────────────────────────────────────────

if [ "${DEPS_ONLY}" = "1" ]; then
    step "Checking what utuimp needs…"
    check_deps "${BUILD} ${RUNTIME}" "${OPTIONAL}"
    if [ -n "${STILL_MISSING}" ]; then
        for n in ${STILL_MISSING}; do warn "still missing: $n: $(where "$n")"; done
        die "Install the above and run the build again."
    fi
    exit 0
fi

# ── header ────────────────────────────────────────────────────────────────────

printf '\n%s    ♪  utuimp installer%s\n' "${BOLD}${MINT}" "${RESET}"
info "https://github.com/${REPO}  ·  ${OS}/${ARCH}  ·  ${PM:-no package manager found}"

# ── is there a release for this system? ───────────────────────────────────────

VERSION=""
if [ "${FROM_SOURCE}" = "0" ]; then
    case "${OS}/${ARCH}" in
        linux/amd64|darwin/amd64|darwin/arm64)
            VERSION="$(fetch "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
                | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/' || true)" ;;
    esac
fi

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT INT TERM

install_release() {
    ARCHIVE="${BIN}_${OS}_${ARCH}.tar.gz"
    BASE_URL="https://github.com/${REPO}/releases/download/${VERSION}"
    step "Downloading ${BIN} ${VERSION}…"
    download "${BASE_URL}/${ARCHIVE}" "${TMP}/${ARCHIVE}" || return 1
    download "${BASE_URL}/checksums.txt" "${TMP}/checksums.txt" || return 1
    EXPECTED="$(awk -v f="${ARCHIVE}" '$2 == f || $2 == "*" f { print $1; exit }' "${TMP}/checksums.txt")"
    ACTUAL=""
    if have sha256sum; then ACTUAL="$(sha256sum "${TMP}/${ARCHIVE}" | awk '{print $1}')"
    elif have shasum; then ACTUAL="$(shasum -a 256 "${TMP}/${ARCHIVE}" | awk '{print $1}')"; fi
    [ -n "${ACTUAL}" ] || die "No sha256 tool found (sha256sum or shasum): refusing to install an unverified binary."
    [ -n "${EXPECTED}" ] || die "No checksum published for ${ARCHIVE}: refusing to install."
    [ "${ACTUAL}" = "${EXPECTED}" ] || die "Checksum mismatch: the download may be corrupted. Please retry."
    success "Checksum OK"
    tar -xzf "${TMP}/${ARCHIVE}" -C "${TMP}"
    mkdir -p "${INSTALL_DIR}"
    cp "${TMP}/${BIN}" "${INSTALL_DIR}/${BIN}"
    chmod 755 "${INSTALL_DIR}/${BIN}"
}

install_source() {
    step "Checking what the build needs…"
    check_deps "git ${BUILD} ${RUNTIME}" "${OPTIONAL}"
    if [ -n "${STILL_MISSING}" ]; then
        for n in ${STILL_MISSING}; do warn "still missing: $n: $(where "$n")"; done
        die "Install the above and run this script again."
    fi
    step "Building utuimp from source…"
    git clone --depth 1 "https://github.com/${REPO}" "${TMP}/src" >/dev/null 2>&1 || die "git clone failed."
    (cd "${TMP}/src" && go build -o "${BIN}" .) || die "The build failed."
    mkdir -p "${INSTALL_DIR}"
    cp "${TMP}/src/${BIN}" "${INSTALL_DIR}/${BIN}"
    chmod 755 "${INSTALL_DIR}/${BIN}"
}

if [ -n "${VERSION}" ]; then
    step "Checking what utuimp needs…"
    check_deps "${RUNTIME}" "${OPTIONAL}"
    for n in ${STILL_MISSING}; do warn "still missing: $n ($(why "$n")): $(where "$n")"; done
    install_release || { warn "The release could not be installed; building from source instead."; install_source; }
else
    info "No published release for ${OS}/${ARCH}: building from source."
    install_source
fi
success "${INSTALL_DIR}/${BIN}"

# The app menu entry and icon (Linux and macOS); harmless when it can't.
"${INSTALL_DIR}/${BIN}" setup-desktop >/dev/null 2>&1 || true

# ── PATH ──────────────────────────────────────────────────────────────────────

case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *)
        step "Adding ${INSTALL_DIR} to PATH…"
        case "$(basename "${SHELL:-sh}")" in
            zsh)  PROFILE="${ZDOTDIR:-${HOME}}/.zshrc"; LINE="export PATH=\"${INSTALL_DIR}:\${PATH}\"" ;;
            bash) PROFILE="${HOME}/.bashrc"; [ -f "${HOME}/.bash_profile" ] && PROFILE="${HOME}/.bash_profile"
                  LINE="export PATH=\"${INSTALL_DIR}:\${PATH}\"" ;;
            fish) PROFILE="${HOME}/.config/fish/config.fish"; LINE="fish_add_path ${INSTALL_DIR}" ;;
            *)    PROFILE="${HOME}/.profile"; LINE="export PATH=\"${INSTALL_DIR}:\${PATH}\"" ;;
        esac
        if ! grep -qF "${LINE}" "${PROFILE}" 2>/dev/null; then
            mkdir -p "$(dirname "${PROFILE}")"
            printf '\n# Added by the utuimp installer\n%s\n' "${LINE}" >> "${PROFILE}"
        fi
        info "Added to ${PROFILE}. Restart your terminal (or: source ${PROFILE})."
        ;;
esac

printf '\n%s    ♪  utuimp is installed.%s Run %sutuimp%s to start.\n\n' "${BOLD}${MINT}" "${RESET}" "${BOLD}" "${RESET}"
