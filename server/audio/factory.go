package audio

import (
	"context"
	"fmt"
	"strings"
)

type DefaultFactory struct{}

func NewDefaultFactory() *DefaultFactory {
	return &DefaultFactory{}
}

func (f *DefaultFactory) CreateFromURL(ctx context.Context, url string, startTimeMs int64) (Source, error) {
	if strings.HasPrefix(url, "tts://") {
		return NewTTSSource(ctx, url, startTimeMs)
	}

	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		ip, err := ValidateHost(url)
		if err != nil {
			return nil, err
		}
		return NewMP3Source(ctx, url, ip, startTimeMs)
	}

	return nil, fmt.Errorf("unsupported URL scheme: %s", url)
}
