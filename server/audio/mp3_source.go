package audio

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/tosone/minimp3"
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
	url       string
	body      io.ReadCloser
	decoder   *minimp3.Decoder
	pcmReader io.Reader
	encoder   *opus.Encoder

	pcmBuffer    []byte
	inputSamples []int16
	pcmSamples   []int16
	opusBuffer   []byte

	srcSampleRate int
	srcChannels   int
	resampleRatio float64
	duration      int64

	position atomic.Int64
	closed   atomic.Bool
	mutex    sync.Mutex
}

func NewMP3Source(ctx context.Context, urlStr string, startTimeMs int64) (*MP3Source, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	switch parsedURL.Scheme {
	case "http":
		if !config.HTTPEnabled {
			return nil, fmt.Errorf("http scheme is disabled")
		}
	case "https":
		if !config.HTTPSEnabled {
			return nil, fmt.Errorf("https scheme is disabled")
		}
	default:
		return nil, fmt.Errorf("unsupported URL scheme: %s", parsedURL.Scheme)
	}

	host := parsedURL.Hostname()
	if host == "" {
		return nil, fmt.Errorf("empty hostname")
	}

	if err := validateHost(host); err != nil {
		return nil, fmt.Errorf("invalid host: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
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

	rawProbe := make([]byte, 4096)
	rn, err := io.ReadFull(resp.Body, rawProbe)
	if err != nil && err != io.ErrUnexpectedEOF {
		resp.Body.Close()
		return nil, fmt.Errorf("read initial data: %w", err)
	}
	rawProbe = rawProbe[:rn]

	xingFrames := parseXingFrames(rawProbe)

	reader := &prefixedReadCloser{
		Reader: io.MultiReader(bytes.NewReader(rawProbe), resp.Body),
		closer: resp.Body,
	}

	source, err := NewMP3SourceFromReader(reader, urlStr, startTimeMs)
	if err != nil {
		return nil, err
	}

	if xingFrames > 0 && source.srcSampleRate > 0 {
		samplesPerFrame := int64(1152) // MPEG1 Layer III
		if source.srcSampleRate < 32000 {
			samplesPerFrame = 576
		}
		source.duration = xingFrames * samplesPerFrame * 1000 / int64(source.srcSampleRate)
	} else if resp.ContentLength > 0 && source.decoder.Kbps > 0 {
		source.duration = resp.ContentLength * 8 / int64(source.decoder.Kbps)
	}

	return source, nil
}

type prefixedReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r *prefixedReadCloser) Close() error {
	return r.closer.Close()
}

func parseXingFrames(data []byte) int64 {
	for _, tag := range []string{"Xing", "Info"} {
		idx := bytes.Index(data, []byte(tag))
		if idx < 0 || idx+8 > len(data) {
			continue
		}
		flags := binary.BigEndian.Uint32(data[idx+4:])
		offset := idx + 8
		if flags&1 != 0 {
			if offset+4 > len(data) {
				continue
			}
			return int64(binary.BigEndian.Uint32(data[offset:]))
		}
	}

	if idx := bytes.Index(data, []byte("VBRI")); idx >= 0 && idx+18 <= len(data) {
		return int64(binary.BigEndian.Uint32(data[idx+14:]))
	}

	return 0
}

func validateHost(host string) error {
	if config.PrivateIPAddressEnabled && config.PublicIPAddressEnabled {
		return nil
	}

	if strings.ToLower(host) == "localhost" {
		if !config.PrivateIPAddressEnabled {
			return fmt.Errorf("localhost not allowed")
		}
		return nil
	}

	ip := net.ParseIP(host)
	if ip == nil {
		ips, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("failed to resolve host: %w", err)
		}
		if len(ips) == 0 {
			return fmt.Errorf("no IPs resolved for host")
		}
		ip = ips[0]
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		if !config.PrivateIPAddressEnabled {
			return fmt.Errorf("private IP address not allowed")
		}
		return nil
	}

	if !config.PublicIPAddressEnabled {
		return fmt.Errorf("public IP address not allowed")
	}

	return nil
}

func NewMP3SourceFromReader(reader io.ReadCloser, url string, startTimeMs int64) (*MP3Source, error) {
	decoder, err := minimp3.NewDecoder(reader)
	if err != nil {
		reader.Close()
		return nil, fmt.Errorf("create mp3 decoder: %w", err)
	}

	probe := make([]byte, 4096)
	n, err := decoder.Read(probe)
	if err != nil && err != io.EOF {
		decoder.Close()
		reader.Close()
		return nil, fmt.Errorf("probe mp3 stream: %w", err)
	}
	if n == 0 {
		decoder.Close()
		reader.Close()
		return nil, fmt.Errorf("empty mp3 stream")
	}

	srcSampleRate := decoder.SampleRate
	srcChannels := decoder.Channels
	if srcChannels < 1 || srcChannels > 2 {
		decoder.Close()
		reader.Close()
		return nil, fmt.Errorf("unsupported channel count: %d", srcChannels)
	}

	resampleRatio := float64(opusSampleRate) / float64(srcSampleRate)

	inputSamplesPerChannel := int(float64(opusFrameSize) / resampleRatio)
	if inputSamplesPerChannel < 1 {
		inputSamplesPerChannel = 1
	}

	inputFrameBytes := inputSamplesPerChannel * srcChannels * 2
	sampleAlign := srcChannels * 2
	inputFrameBytes = ((inputFrameBytes + sampleAlign - 1) / sampleAlign) * sampleAlign

	encoder, err := opus.NewEncoder(opusSampleRate, opusChannels, opus.AppAudio)
	if err != nil {
		decoder.Close()
		reader.Close()
		return nil, fmt.Errorf("create opus encoder: %w", err)
	}

	pcmReader := io.MultiReader(bytes.NewReader(probe[:n]), decoder)

	source := &MP3Source{
		url:           url,
		body:          reader,
		decoder:       decoder,
		pcmReader:     pcmReader,
		encoder:       encoder,
		pcmBuffer:     make([]byte, inputFrameBytes),
		inputSamples:  make([]int16, inputSamplesPerChannel*opusChannels),
		pcmSamples:    make([]int16, opusFrameSize*opusChannels),
		opusBuffer:    make([]byte, 4000),
		srcSampleRate: srcSampleRate,
		srcChannels:   srcChannels,
		resampleRatio: resampleRatio,
	}

	source.position.Store(startTimeMs)

	return source, nil
}

func (s *MP3Source) ProvideOpusFrame() ([]byte, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.closed.Load() {
		return nil, io.EOF
	}

	n, err := io.ReadFull(s.pcmReader, s.pcmBuffer)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			if n == 0 {
				return nil, io.EOF
			}
			clear(s.pcmBuffer[n:])
		} else {
			return nil, fmt.Errorf("read pcm: %w", err)
		}
	}

	numSamplesPerChannel := len(s.pcmBuffer) / (s.srcChannels * 2)
	if s.srcChannels == 1 {
		for i := range numSamplesPerChannel {
			sample := int16(binary.LittleEndian.Uint16(s.pcmBuffer[i*2:]))
			s.inputSamples[i*2] = sample
			s.inputSamples[i*2+1] = sample
		}
	} else {
		numSamples := len(s.pcmBuffer) / 2
		for i := range numSamples {
			s.inputSamples[i] = int16(binary.LittleEndian.Uint16(s.pcmBuffer[i*2:]))
		}
	}

	if s.resampleRatio != 1.0 {
		s.resampleLinear(s.inputSamples, s.pcmSamples)
	} else {
		copy(s.pcmSamples, s.inputSamples)
	}

	numBytes, err := s.encoder.Encode(s.pcmSamples, s.opusBuffer)
	if err != nil {
		return nil, fmt.Errorf("encode opus: %w", err)
	}

	s.position.Add(20)

	return s.opusBuffer[:numBytes], nil
}

func (s *MP3Source) resampleLinear(input, output []int16) {
	inputLen := len(input) / opusChannels
	outputLen := len(output) / opusChannels

	for i := range outputLen {
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

		for ch := range opusChannels {
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
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.decoder.Close()
	s.body.Close()
	s.decoder = nil
	s.pcmReader = nil
	s.body = nil
}

func (s *MP3Source) Position() int64 {
	return s.position.Load()
}

func (s *MP3Source) SeekTo(positionMs int64) error {
	return errors.New("seek not supported for HTTP streams")
}

func (s *MP3Source) Duration() int64 {
	return s.duration
}

func (s *MP3Source) CanSeek() bool {
	return false
}

func (s *MP3Source) URL() string {
	return s.url
}
