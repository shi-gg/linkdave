package audio

import (
	"context"
	"io"
)

type Source interface {
	ProvideOpusFrame() ([]byte, error)
	Close()
	Position() int64
	SeekTo(positionMs int64) error
	Duration() int64
	CanSeek() bool
}

type SourceFactory interface {
	CreateFromURL(ctx context.Context, url string, startTimeMs int64) (Source, error)
}

var ErrEOF = io.EOF
