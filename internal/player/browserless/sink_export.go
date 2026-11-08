package browserless

import "time"

// AudioSink is the platform audio output (GStreamer on Linux, Core Audio on
// macOS, Media Foundation on Windows). It plays MP3 or ADTS AAC from a
// loopback http:// URL; the format is taken from the URL's extension
// (.mp3 / .aac).
type AudioSink interface {
	PlayURI(uri string)
	SetNextURI(uri string)
	Play()
	Pause()
	Stop()
	Seek(d time.Duration)
	SetVolume(v float64)
	SetEqualizer(gains []float64)
	Position() time.Duration
	OnEOS(fn func())
	OnError(fn func(error))
	OnStreamStart(fn func(uri string))
	Destroy()
}

// NewAudioSink opens the platform audio output for other players.
func NewAudioSink() (AudioSink, error) { return newAudioSink() }
