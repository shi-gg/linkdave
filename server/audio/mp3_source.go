package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/hajimehoshi/go-mp3"
	"gopkg.in/hraban/opus.v2"
)

const (
	// Discord expects 48kHz, stereo, 20ms frames
	opusSampleRate = 48000
	opusChannels   = 2
	opusFrameSize  = 960 // 20ms at 48kHz (48000 * 0.020)
	// PCM bytes needed for one opus frame: 960 samples * 2 channels * 2 bytes/sample
	pcmFrameBytes = opusFrameSize * opusChannels * 2
)

type MP3Source struct {
	url     string
	body    io.ReadCloser
	decoder *mp3.Decoder
	encoder *opus.Encoder

	pcmBuffer  []byte
	pcmSamples []int16
	opusBuffer []byte

	srcSampleRate int
	resampleRatio float64

	position atomic.Int64
	closed   atomic.Bool
	mu       sync.Mutex
}

func NewMP3Source(ctx context.Context, url string, startTimeMs int64) (*MP3Source, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "LinkDave/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch audio: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	decoder, err := mp3.NewDecoder(resp.Body)
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("create mp3 decoder: %w", err)
	}

	encoder, err := opus.NewEncoder(opusSampleRate, opusChannels, opus.AppAudio)
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("create opus encoder: %w", err)
	}

	// go-mp3 outputs 16-bit stereo PCM at source sample rate
	srcSampleRate := decoder.SampleRate()
	resampleRatio := float64(opusSampleRate) / float64(srcSampleRate)

	// If source is 44.1kHz, we need fewer input samples than output samples
	inputFrameBytes := int(float64(pcmFrameBytes) / resampleRatio)
	inputFrameBytes = ((inputFrameBytes + 3) / 4) * 4

	return &MP3Source{
		url:           url,
		body:          resp.Body,
		decoder:       decoder,
		encoder:       encoder,
		pcmBuffer:     make([]byte, inputFrameBytes),
		pcmSamples:    make([]int16, opusFrameSize*opusChannels),
		opusBuffer:    make([]byte, 4000),
		srcSampleRate: srcSampleRate,
		resampleRatio: resampleRatio,
	}, nil
}

func (s *MP3Source) ProvideOpusFrame() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return nil, io.EOF
	}

	// go-mp3 outputs 16-bit little-endian stereo
	n, err := io.ReadFull(s.decoder, s.pcmBuffer)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			if n == 0 {
				return nil, io.EOF
			}
			// Partial read - pad with silence
			clear(s.pcmBuffer[n:])
		} else {
			return nil, fmt.Errorf("read pcm: %w", err)
		}
	}

	numInputSamples := len(s.pcmBuffer) / 2
	inputSamples := make([]int16, numInputSamples)
	for i := 0; i < numInputSamples; i++ {
		inputSamples[i] = int16(binary.LittleEndian.Uint16(s.pcmBuffer[i*2:]))
	}

	// Resample if needed (linear interpolation for simplicity)
	if s.resampleRatio != 1.0 {
		s.resampleLinear(inputSamples, s.pcmSamples)
	} else {
		copy(s.pcmSamples, inputSamples)
	}

	numBytes, err := s.encoder.Encode(s.pcmSamples, s.opusBuffer)
	if err != nil {
		return nil, fmt.Errorf("encode opus: %w", err)
	}

	s.position.Add(20)

	frame := make([]byte, numBytes)
	copy(frame, s.opusBuffer[:numBytes])
	return frame, nil
}

func (s *MP3Source) resampleLinear(input, output []int16) {
	inputLen := len(input) / opusChannels
	outputLen := len(output) / opusChannels

	for i := 0; i < outputLen; i++ {
		srcPos := float64(i) / s.resampleRatio
		srcIdx := int(srcPos)
		frac := srcPos - float64(srcIdx)

		if srcIdx >= inputLen-1 {
			srcIdx = inputLen - 2
			frac = 1.0
		}
		if srcIdx < 0 {
			srcIdx = 0
			frac = 0.0
		}

		for ch := 0; ch < opusChannels; ch++ {
			idx0 := srcIdx*opusChannels + ch
			idx1 := (srcIdx+1)*opusChannels + ch
			if idx1 >= len(input) {
				idx1 = idx0
			}

			sample0 := float64(input[idx0])
			sample1 := float64(input[idx1])
			interpolated := sample0 + frac*(sample1-sample0)
			output[i*opusChannels+ch] = int16(interpolated)
		}
	}
}

func (s *MP3Source) Close() {
	if s.closed.Swap(true) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.body.Close()
}

func (s *MP3Source) Position() int64 {
	return s.position.Load()
}

func (s *MP3Source) SeekTo(positionMs int64) error {
	return errors.New("seek not supported for HTTP streams")
}

func (s *MP3Source) Duration() int64 {
	return 0
}

func (s *MP3Source) CanSeek() bool {
	return false
}
