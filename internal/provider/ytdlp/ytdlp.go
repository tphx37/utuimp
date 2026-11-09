// Package ytdlp runs yt-dlp for the services utuimp plays through it
// (YouTube Music, SoundCloud without an API app): listing pages, resolving
// songs, and streaming the audio as ADTS AAC through ffmpeg.
package ytdlp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// CookieArgs turns the cookies setting into yt-dlp flags: an existing file
// is a cookies.txt, anything else a browser name.
func CookieArgs(cookies string) []string {
	cookies = strings.TrimSpace(cookies)
	switch {
	case cookies == "":
		return nil
	case fileExists(cookies):
		return []string{"--cookies", cookies}
	default:
		return []string{"--cookies-from-browser", cookies}
	}
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// ExecRunner runs the yt-dlp at path with base flags first.
func ExecRunner(path string, base []string) Runner {
	return func(ctx context.Context, args ...string) ([]byte, error) {
		full := append([]string{"--no-warnings", "--ignore-config"}, base...)
		cmd := exec.CommandContext(ctx, path, append(full, args...)...) //nolint:gosec // our own arguments
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, Error(stderr.String(), err)
		}
		return out, nil
	}
}

// ErrBotCheck means YouTube wants proof the request comes from a person:
// yt-dlp needs the browser's cookies.
var ErrBotCheck = errors.New("YouTube asks to sign in to confirm you're not a bot: set youtube_cookies (run utuimp setup)")

// Error turns yt-dlp's stderr into an error: its last ERROR line.
func Error(stderr string, err error) error {
	if strings.Contains(stderr, "not a bot") {
		return ErrBotCheck
	}
	msg := ""
	for _, l := range strings.Split(strings.TrimSpace(stderr), "\n") {
		if strings.HasPrefix(l, "ERROR:") {
			msg = strings.TrimSpace(strings.TrimPrefix(l, "ERROR:"))
		}
	}
	if msg == "" {
		return fmt.Errorf("yt-dlp: %w", err)
	}
	return errors.New("yt-dlp: " + msg)
}

// PickFormat chooses the audio to fetch: the best AAC (remuxed as is), or
// else the best audio of any codec (re-encoded).
func PickFormat(fs []Format) (id string, copyAAC bool, err error) {
	var bestAAC, bestAny *Format
	for i := range fs {
		f := &fs[i]
		if f.Vcodec != "none" || f.Acodec == "" || f.Acodec == "none" {
			continue
		}
		if strings.HasPrefix(f.Acodec, "mp4a") && (bestAAC == nil || f.Abr > bestAAC.Abr) {
			bestAAC = f
		}
		if bestAny == nil || f.Abr > bestAny.Abr {
			bestAny = f
		}
	}
	switch {
	case bestAAC != nil:
		return bestAAC.FormatID, true, nil
	case bestAny != nil:
		return bestAny.FormatID, false, nil
	}
	return "", false, errors.New("no audio stream")
}

// Streamer runs yt-dlp | ffmpeg for one song.
type Streamer struct {
	YtDlp, FFmpeg string
	Base          []string // flags before every call (cookies)
}

func (s Streamer) Open(ctx context.Context, info []byte, formatID string, copyAAC bool) (io.ReadCloser, error) {
	// yt-dlp writes HLS fragments next to its output, which for "-" is the
	// working directory: give it a scratch directory of its own, which also
	// holds the info JSON.
	dir, err := os.MkdirTemp("", "utuimp-ytdlp-*")
	if err != nil {
		return nil, err
	}
	tmp := filepath.Join(dir, "info.json")
	if err := os.WriteFile(tmp, info, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	ytArgs := append([]string{"--no-warnings", "--ignore-config", "-q"}, s.Base...)
	ytArgs = append(ytArgs, "--load-info-json", tmp, "-f", formatID, "--no-part", "-o", "-")
	dl := exec.CommandContext(ctx, s.YtDlp, ytArgs...) //nolint:gosec // our own arguments
	dl.Dir = dir
	codec := []string{"-c:a", "copy"}
	if !copyAAC {
		codec = []string{"-c:a", "aac", "-b:a", "192k"}
	}
	ffArgs := append([]string{"-hide_banner", "-loglevel", "error", "-i", "pipe:0", "-vn"}, codec...)
	ff := exec.CommandContext(ctx, s.FFmpeg, append(ffArgs, "-f", "adts", "pipe:1")...) //nolint:gosec // our own arguments

	var dlErr, ffErr bytes.Buffer
	dl.Stderr, ff.Stderr = &dlErr, &ffErr
	pipe, err := dl.StdoutPipe()
	if err != nil {
		cancel()
		_ = os.RemoveAll(dir)
		return nil, err
	}
	ff.Stdin = pipe
	out, err := ff.StdoutPipe()
	if err != nil {
		cancel()
		_ = os.RemoveAll(dir)
		return nil, err
	}
	if err := dl.Start(); err != nil {
		cancel()
		_ = os.RemoveAll(dir)
		return nil, err
	}
	if err := ff.Start(); err != nil {
		cancel()
		_ = dl.Wait()
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}
	// ffmpeg holds its own copy of the pipe; ours would keep yt-dlp
	// writing after ffmpeg is gone.
	if c, ok := pipe.(io.Closer); ok {
		_ = c.Close()
	}
	return &procReader{r: out, cancel: cancel, cmds: []*exec.Cmd{dl, ff}, errs: []*bytes.Buffer{&dlErr, &ffErr}, tmp: dir}, nil
}

// procReader reads ffmpeg's output and cleans up both processes on Close.
// A pipeline that fails before any audio reports yt-dlp's or ffmpeg's error.
type procReader struct {
	r      io.Reader
	cancel context.CancelFunc
	cmds   []*exec.Cmd
	errs   []*bytes.Buffer
	tmp    string // scratch directory, removed when done
	n      int64
	once   sync.Once
	err    error
}

func (p *procReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.n += int64(n)
	if errors.Is(err, io.EOF) && p.n == 0 {
		if werr := p.wait(); werr != nil {
			return n, werr
		}
	}
	return n, err
}

func (p *procReader) wait() error {
	p.once.Do(func() {
		for i, c := range p.cmds {
			if err := c.Wait(); err != nil && p.err == nil {
				p.err = Error(p.errs[i].String(), err)
			}
		}
		_ = os.RemoveAll(p.tmp)
	})
	return p.err
}

func (p *procReader) Close() error {
	p.cancel()
	_ = p.wait()
	return nil
}

// Runner runs yt-dlp with args and returns its standard output.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// ── yt-dlp JSON ────────────────────────────────────────────────────────────

// Thumb is a thumbnail.
type Thumb struct {
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Entry is a yt-dlp info JSON: a video, a playlist or a flat item.
type Entry struct {
	Type        string   `json:"_type"`
	ID          string   `json:"id"`
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Track       string   `json:"track"`
	Artist      string   `json:"artist"`
	Artists     []string `json:"artists"`
	Album       string   `json:"album"`
	Channel     string   `json:"channel"`
	ChannelID   string   `json:"channel_id"`
	Uploader    string   `json:"uploader"`
	UploaderURL string   `json:"uploader_url"`
	WebpageURL  string   `json:"webpage_url"`
	Duration    float64  `json:"duration"`
	ReleaseYear int      `json:"release_year"`
	ReleaseDate string   `json:"release_date"` // YYYYMMDD
	Thumbnail   string   `json:"thumbnail"`
	Thumbnails  []Thumb  `json:"thumbnails"`
	Entries     []Entry  `json:"entries"`
	Formats     []Format `json:"formats"`
}

// Format is one downloadable format.
type Format struct {
	FormatID string  `json:"format_id"`
	Acodec   string  `json:"acodec"`
	Vcodec   string  `json:"vcodec"`
	Abr      float64 `json:"abr"`
}

// BestThumb picks a thumbnail about 400px wide.
func BestThumb(ts []Thumb) string {
	best, bw := "", 0
	for _, t := range ts {
		if t.URL == "" {
			continue
		}
		if best == "" || (t.Width >= 300 && (bw < 300 || t.Width < bw)) || (bw < 300 && t.Width > bw) {
			best, bw = t.URL, t.Width
		}
	}
	return best
}

// Flat lists u without resolving each item; limit 0 means all.
func Flat(ctx context.Context, run Runner, u string, limit int) (*Entry, error) {
	args := []string{"--flat-playlist", "-J"}
	if limit > 0 {
		args = append(args, "--playlist-end", fmt.Sprint(limit))
	}
	out, err := run(ctx, append(args, u)...)
	if err != nil {
		return nil, err
	}
	var e Entry
	if err := json.Unmarshal(out, &e); err != nil {
		return nil, fmt.Errorf("yt-dlp output: %w", err)
	}
	return &e, nil
}

// Info resolves one item fully (formats and all metadata). The raw JSON is
// returned too, for Streamer.Open.
func Info(ctx context.Context, run Runner, u string) (*Entry, []byte, error) {
	out, err := run(ctx, "-J", "--no-playlist", u)
	if err != nil {
		return nil, nil, err
	}
	var e Entry
	if err := json.Unmarshal(out, &e); err != nil {
		return nil, nil, fmt.Errorf("yt-dlp output: %w", err)
	}
	return &e, out, nil
}
