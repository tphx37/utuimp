package cmd

import (
	"errors"
	"fmt"
	"os/exec"

	tea "charm.land/bubbletea/v2"

	"github.com/tphx37/utuimp/internal/config"
	"github.com/tphx37/utuimp/internal/i18n"
	"github.com/tphx37/utuimp/internal/player/ystream"
	"github.com/tphx37/utuimp/internal/provider/localstore"
	"github.com/tphx37/utuimp/internal/provider/ytmusic"
	"github.com/tphx37/utuimp/internal/tui"
)

// findTool returns the configured path, or name looked up on PATH.
func findTool(configured, name string) (string, error) {
	if configured != "" {
		return exec.LookPath(configured)
	}
	return exec.LookPath(name)
}

// ytInstallHint says how to get yt-dlp and ffmpeg.
func ytInstallHint() string {
	return i18n.T("install yt-dlp and ffmpeg (Arch: sudo pacman -S yt-dlp ffmpeg; others: pipx install yt-dlp)")
}

// startYouTubeEngine brings up YouTube Music: yt-dlp lists and fetches the
// music, ffmpeg remuxes it, the shared stream player plays it.
func startYouTubeEngine(cfg *config.Config, send func(tea.Msg), env *engineEnv, idx int) {
	send(tui.InitStatusMsg(i18n.T("Starting YouTube Music…")))
	ytdlp, err := findTool(cfg.YtDlpPath, "yt-dlp")
	if err != nil {
		send(tui.InitErrMsg{Err: errors.New(i18n.T("YouTube Music needs yt-dlp: ") + ytInstallHint())})
		return
	}
	ffmpeg, err := findTool(cfg.FFmpegPath, "ffmpeg")
	if err != nil {
		send(tui.InitErrMsg{Err: errors.New(i18n.T("YouTube Music needs ffmpeg: ") + ytInstallHint())})
		return
	}
	store, err := localstore.Open(localstore.DefaultPath("library-youtube"))
	if err != nil {
		send(tui.InitErrMsg{Err: fmt.Errorf("youtube music library: %w", err)})
		return
	}
	prov := ytmusic.New(ytdlp, ffmpeg, cfg.YouTubeCookies, store)
	plyr, err := ystream.New(prov, func(msg string) { send(tui.DebugLogMsg(msg)) }, ystream.WithName("youtube"))
	if err != nil {
		send(tui.InitErrMsg{Err: fmt.Errorf("audio engine: %w", err)})
		return
	}
	env.addCleanup(func() { _ = plyr.Close() })
	env.mux.Set(idx, plyr)
	send(tui.EngineReadyMsg{Player: plyr, Provider: prov, Backend: "YouTube Music · yt-dlp · GStreamer"})
}
