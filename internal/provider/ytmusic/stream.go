package ytmusic

import (
	"context"
	"errors"
	"io"

	"github.com/tphx37/utuimp/internal/provider"
	"github.com/tphx37/utuimp/internal/provider/ytdlp"
)

// StreamURL is not used: the stream comes from OpenStream.
func (p *Provider) StreamURL(context.Context, string) (string, string, error) {
	return "", "", errors.New("youtube music streams through OpenStream")
}

// OpenStream implements ystream.Opener: it resolves the song once (which
// also gives its full metadata), then pipes the chosen audio through ffmpeg
// as ADTS AAC.
func (p *Provider) OpenStream(ctx context.Context, id string) (io.ReadCloser, string, *provider.Track, error) {
	e, info, err := ytdlp.Info(ctx, p.run, watchURL(id))
	if err != nil {
		return nil, "", nil, err
	}
	fid, copyAAC, err := ytdlp.PickFormat(e.Formats)
	if err != nil {
		return nil, "", nil, err
	}
	t := p.toTrack(*e)
	rc, err := p.stream.Open(ctx, info, fid, copyAAC)
	if err != nil {
		return nil, "", nil, err
	}
	return rc, "aac", &t, nil
}
