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
	ip, err := ValidateHost(url)
	if err != nil {
		return nil, err
	}

	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return NewMP3Source(ctx, url, ip, startTimeMs)
	}
	return nil, fmt.Errorf("unsupported URL scheme: %s", url)
}
