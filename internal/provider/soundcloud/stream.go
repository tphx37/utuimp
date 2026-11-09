package soundcloud

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/tphx37/utuimp/internal/provider"
)

type streams struct {
	HTTPMP3 string `json:"http_mp3_128_url"`
	HLSMP3  string `json:"hls_mp3_128_url"`
	HLSAAC  string `json:"hls_aac_160_url"`
}

// StreamURL is not used: the stream comes from OpenStream.
func (p *Provider) StreamURL(context.Context, string) (string, string, error) {
	return "", "", errors.New("soundcloud streams through OpenStream")
}

// OpenStream implements ystream.Opener. SoundCloud streams are HLS: the MP3
// one is played by joining its segments (MP3 frames need no container);
// the AAC one, when it is all there is, is remuxed by ffmpeg.
func (p *Provider) OpenStream(ctx context.Context, id string) (io.ReadCloser, string, *provider.Track, error) {
	var s streams
	if err := p.c.get(ctx, "/tracks/"+id+"/streams", nil, &s); err != nil {
		return nil, "", nil, err
	}
	switch {
	case s.HTTPMP3 != "":
		resp, err := p.c.do(ctx, http.MethodGet, s.HTTPMP3, nil, nil)
		if err != nil {
			return nil, "", nil, err
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, "", nil, fmt.Errorf("soundcloud stream: HTTP %d", resp.StatusCode)
		}
		return resp.Body, "mp3", nil, nil
	case s.HLSMP3 != "":
		segs, err := p.playlist(ctx, s.HLSMP3)
		if err != nil {
			return nil, "", nil, err
		}
		return &hlsReader{ctx: ctx, c: p.c, segs: segs}, "mp3", nil, nil
	case s.HLSAAC != "" && p.ffmpeg != "":
		rc, err := p.aacViaFFmpeg(ctx, s.HLSAAC)
		return rc, "aac", nil, err
	case s.HLSAAC != "":
		return nil, "", nil, errors.New("soundcloud: this track only streams as AAC, which needs ffmpeg")
	}
	return nil, "", nil, errNoStream
}

// playlist fetches an HLS media playlist and returns its segment URLs.
func (p *Provider) playlist(ctx context.Context, u string) ([]string, error) {
	resp, err := p.c.do(ctx, http.MethodGet, u, nil, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("soundcloud playlist: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	return parseM3U8(body, resp.Request.URL)
}

// parseM3U8 lists the segment URLs of a media playlist, resolved against
// base.
func parseM3U8(body []byte, base *url.URL) ([]string, error) {
	var segs []string
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case line == "":
		case strings.HasPrefix(line, "#EXT-X-KEY") && !strings.Contains(line, "METHOD=NONE"):
			return nil, errors.New("soundcloud: the stream is encrypted")
		case strings.HasPrefix(line, "#"):
		default:
			ref, err := url.Parse(line)
			if err != nil {
				return nil, err
			}
			segs = append(segs, base.ResolveReference(ref).String())
		}
	}
	if len(segs) == 0 {
		return nil, errors.New("soundcloud: empty stream playlist")
	}
	return segs, nil
}

// hlsReader reads the segments one after another.
type hlsReader struct {
	ctx  context.Context
	c    *Client
	segs []string
	cur  io.ReadCloser
}

func (h *hlsReader) Read(b []byte) (int, error) {
	for {
		if h.cur == nil {
			if len(h.segs) == 0 {
				return 0, io.EOF
			}
			resp, err := h.c.do(h.ctx, http.MethodGet, h.segs[0], nil, nil)
			if err != nil {
				return 0, err
			}
			if resp.StatusCode != http.StatusOK {
				_ = resp.Body.Close()
				return 0, fmt.Errorf("soundcloud segment: HTTP %d", resp.StatusCode)
			}
			h.segs, h.cur = h.segs[1:], resp.Body
		}
		n, err := h.cur.Read(b)
		if errors.Is(err, io.EOF) {
			_ = h.cur.Close()
			h.cur = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func (h *hlsReader) Close() error {
	if h.cur != nil {
		return h.cur.Close()
	}
	return nil
}

// aacViaFFmpeg remuxes the AAC HLS stream to ADTS. The playlist is fetched
// here (with the token) and handed to ffmpeg as a file with absolute
// segment links.
func (p *Provider) aacViaFFmpeg(ctx context.Context, u string) (io.ReadCloser, error) {
	resp, err := p.c.do(ctx, http.MethodGet, u, nil, nil)
	if err != nil {
		return nil, err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("soundcloud playlist: HTTP %d", resp.StatusCode)
	}
	base := resp.Request.URL
	var out bytes.Buffer
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			if ref, err := url.Parse(line); err == nil {
				line = base.ResolveReference(ref).String()
			}
		}
		// EXT-X-MAP URIs are relative too.
		if i := strings.Index(line, `URI="`); i >= 0 && strings.HasPrefix(line, "#EXT-X-MAP") {
			rest := line[i+5:]
			if j := strings.IndexByte(rest, '"'); j >= 0 {
				if ref, err := url.Parse(rest[:j]); err == nil {
					line = line[:i+5] + base.ResolveReference(ref).String() + rest[j:]
				}
			}
		}
		out.WriteString(line + "\n")
	}
	f, err := os.CreateTemp("", "utuimp-sc-*.m3u8")
	if err != nil {
		return nil, err
	}
	_, _ = f.Write(out.Bytes())
	_ = f.Close()

	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, p.ffmpeg, "-hide_banner", "-loglevel", "error", //nolint:gosec // our own arguments
		"-protocol_whitelist", "file,http,https,tcp,tls,crypto",
		"-i", f.Name(), "-vn", "-c:a", "copy", "-f", "adts", "pipe:1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		cancel()
		_ = os.Remove(f.Name())
		return nil, fmt.Errorf("ffmpeg: %w", err)
	}
	return &cmdReader{r: pipe, cmd: cmd, cancel: cancel, tmp: f.Name(), stderr: &stderr}, nil
}

type cmdReader struct {
	r      io.Reader
	cmd    *exec.Cmd
	cancel context.CancelFunc
	tmp    string
	stderr *bytes.Buffer
	n      int
	closed bool
}

func (c *cmdReader) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.n += n
	if errors.Is(err, io.EOF) && c.n == 0 {
		if werr := c.wait(); werr != nil {
			return n, fmt.Errorf("ffmpeg: %s", strings.TrimSpace(c.stderr.String()))
		}
	}
	return n, err
}

func (c *cmdReader) wait() error {
	if c.closed {
		return nil
	}
	c.closed = true
	err := c.cmd.Wait()
	_ = os.Remove(c.tmp)
	return err
}

func (c *cmdReader) Close() error {
	c.cancel()
	_ = c.wait()
	return nil
}
