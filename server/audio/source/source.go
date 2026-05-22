package source

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/shi-gg/linkdave/server/audio/filter"
)

type Source interface {
	ProvideOpusFrame() ([]byte, error)
	Close()
	Position() int64
	SeekTo(positionMs int64) error
	Duration() int64
	CanSeek() bool
	URL() string
}

var ErrEOF = io.EOF

type DefaultFactory struct{}

func NewDefaultFactory() *DefaultFactory {
	return &DefaultFactory{}
}

func (f *DefaultFactory) CreateFromURL(ctx context.Context, url string, startTimeMs int64, filters *filter.Filters) (Source, error) {
	if strings.HasPrefix(url, "tts://") {
		if !cfg.TextToSpeechEnabled {
			return nil, fmt.Errorf("tts scheme is disabled")
		}
		return NewTTSSource(ctx, url, startTimeMs)
	}

	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		ip, err := ValidateHost(url)
		if err != nil {
			return nil, err
		}
		return NewMP3Source(ctx, url, ip, startTimeMs, filters)
	}

	return nil, fmt.Errorf("unsupported URL scheme: %s", url)
}
