package ytmusic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/tphx37/utuimp/internal/provider"
)

// cookieArgs turns the cookies setting into yt-dlp flags: an existing file
// is a cookies.txt, anything else a browser name.
func cookieArgs(cookies string) []string {
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

// execRunner runs the yt-dlp at path with base flags first.
func execRunner(path string, base []string) Runner {
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
			return nil, ytdlpError(stderr.String(), err)
		}
		return out, nil
	}
}

// ErrBotCheck means YouTube wants proof the request comes from a person:
// yt-dlp needs the browser's cookies.
var ErrBotCheck = errors.New("YouTube asks to sign in to confirm you're not a bot: set youtube_cookies (run utuimp setup)")

// ytdlpError turns yt-dlp's stderr into an error: its last ERROR line.
func ytdlpError(stderr string, err error) error {
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

// pickFormat chooses the audio to fetch: the best AAC (remuxed as is), or
// else the best audio of any codec (re-encoded).
func pickFormat(fs []format) (id string, copyAAC bool, err error) {
	var bestAAC, bestAny *format
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

// streamer runs yt-dlp | ffmpeg for one song.
type streamer struct {
	ytdlp, ffmpeg string
	base          []string
	run           Runner
}

// StreamURL is not used: the stream comes from OpenStream.
func (p *Provider) StreamURL(context.Context, string) (string, string, error) {
	return "", "", errors.New("youtube music streams through OpenStream")
}

// OpenStream implements ystream.Opener: it resolves the song once (which
// also gives its full metadata), then pipes the chosen audio through ffmpeg
// as ADTS AAC.
func (p *Provider) OpenStream(ctx context.Context, id string) (io.ReadCloser, string, *provider.Track, error) {
	info, err := p.run(ctx, "-J", "--no-playlist", watchURL(id))
	if err != nil {
		return nil, "", nil, err
	}
	var e entry
	if err := json.Unmarshal(info, &e); err != nil {
		return nil, "", nil, fmt.Errorf("yt-dlp output: %w", err)
	}
	fid, copyAAC, err := pickFormat(e.Formats)
	if err != nil {
		return nil, "", nil, err
	}
	t := p.toTrack(e)
	rc, err := p.stream.open(ctx, info, fid, copyAAC)
	if err != nil {
		return nil, "", nil, err
	}
	return rc, "aac", &t, nil
}

func (s streamer) open(ctx context.Context, info []byte, formatID string, copyAAC bool) (io.ReadCloser, error) {
	f, err := os.CreateTemp("", "utuimp-yt-*.json")
	if err != nil {
		return nil, err
	}
	tmp := f.Name()
	if _, err := f.Write(info); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return nil, err
	}
	_ = f.Close()

	ctx, cancel := context.WithCancel(ctx)
	ytArgs := append([]string{"--no-warnings", "--ignore-config", "-q"}, s.base...)
	ytArgs = append(ytArgs, "--load-info-json", tmp, "-f", formatID, "--no-part", "-o", "-")
	dl := exec.CommandContext(ctx, s.ytdlp, ytArgs...) //nolint:gosec // our own arguments
	codec := []string{"-c:a", "copy"}
	if !copyAAC {
		codec = []string{"-c:a", "aac", "-b:a", "192k"}
	}
	ffArgs := append([]string{"-hide_banner", "-loglevel", "error", "-i", "pipe:0", "-vn"}, codec...)
	ff := exec.CommandContext(ctx, s.ffmpeg, append(ffArgs, "-f", "adts", "pipe:1")...) //nolint:gosec // our own arguments

	var dlErr, ffErr bytes.Buffer
	dl.Stderr, ff.Stderr = &dlErr, &ffErr
	pipe, err := dl.StdoutPipe()
	if err != nil {
		cancel()
		_ = os.Remove(tmp)
		return nil, err
	}
	ff.Stdin = pipe
	out, err := ff.StdoutPipe()
	if err != nil {
		cancel()
		_ = os.Remove(tmp)
		return nil, err
	}
	if err := dl.Start(); err != nil {
		cancel()
		_ = os.Remove(tmp)
		return nil, err
	}
	if err := ff.Start(); err != nil {
		cancel()
		_ = dl.Wait()
		_ = os.Remove(tmp)
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}
	// ffmpeg holds its own copy of the pipe; ours would keep yt-dlp
	// writing after ffmpeg is gone.
	if c, ok := pipe.(io.Closer); ok {
		_ = c.Close()
	}
	return &procReader{r: out, cancel: cancel, cmds: []*exec.Cmd{dl, ff}, errs: []*bytes.Buffer{&dlErr, &ffErr}, tmp: tmp}, nil
}

// procReader reads ffmpeg's output and cleans up both processes on Close.
// A pipeline that fails before any audio reports yt-dlp's or ffmpeg's error.
type procReader struct {
	r      io.Reader
	cancel context.CancelFunc
	cmds   []*exec.Cmd
	errs   []*bytes.Buffer
	tmp    string
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
				p.err = ytdlpError(p.errs[i].String(), err)
			}
		}
		_ = os.Remove(p.tmp)
	})
	return p.err
}

func (p *procReader) Close() error {
	p.cancel()
	_ = p.wait()
	return nil
}
