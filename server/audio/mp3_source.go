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
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/tosone/minimp3"
	"gopkg.in/hraban/opus.v2"
)

const (
	OPUS_SAMPLE_RATE       = 48000 // Discord expects 48kHz, stereo, 20ms frames
	OPUS_CHANNELS          = 2
	OPUS_FRAME_SIZE        = 960                                       // 20ms at 48kHz (48000 * 0.020)
	OPUS_FRAME_DURATION_MS = OPUS_FRAME_SIZE * 1000 / OPUS_SAMPLE_RATE // 20ms
	OPUS_MAX_FRAME_BYTES   = 4000

	IDLE_TIMEOUT       = 10 * time.Second
	DIAL_TIMEOUT       = 5 * time.Second
	KEEPALIVE_INTERVAL = 30 * time.Second

	PROBE_SIZE = 4096

	MPEG1_SAMPLES_PER_FRAME     = 1152
	MPEG2_SAMPLES_PER_FRAME     = 576
	MPEG2_SAMPLE_RATE_THRESHOLD = 32000

	XING_FLAGS_OFFSET     = 4
	XING_DATA_OFFSET      = 8
	XING_FRAME_COUNT_SIZE = 4

	VBRI_FRAME_COUNT_OFFSET = 14
	VBRI_MIN_SIZE           = 18

	BITS_PER_BYTE = 8
)

var baseTransport = &http.Transport{
	ResponseHeaderTimeout: DIAL_TIMEOUT,
	DialContext: (&net.Dialer{
		Timeout:   DIAL_TIMEOUT,
		KeepAlive: KEEPALIVE_INTERVAL,
	}).DialContext,
}

var clientByIP sync.Map

func clientForIP(ip string) *http.Client {
	if v, ok := clientByIP.Load(ip); ok {
		return v.(*http.Client)
	}

	dialer := &net.Dialer{
		Timeout:   DIAL_TIMEOUT,
		KeepAlive: KEEPALIVE_INTERVAL,
	}

	transport := baseTransport.Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse address %q: %w", addr, err)
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
	}

	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	if clientForIp, loaded := clientByIP.LoadOrStore(ip, client); loaded {
		return clientForIp.(*http.Client)
	}

	return client
}

type idleTimeoutReader struct {
	body  io.ReadCloser
	timer *time.Timer
}

func newIdleTimeoutReader(body io.ReadCloser) *idleTimeoutReader {
	r := &idleTimeoutReader{body: body}
	r.timer = time.AfterFunc(IDLE_TIMEOUT, func() { _ = body.Close() })
	return r
}

func (r *idleTimeoutReader) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if n > 0 {
		r.timer.Reset(IDLE_TIMEOUT)
	}
	return n, err
}

func (r *idleTimeoutReader) Close() error {
	r.timer.Stop()
	return r.body.Close()
}

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

func NewMP3Source(ctx context.Context, urlStr, ip string, startTimeMs int64) (*MP3Source, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", parsedURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "LinkDave/1.0")

	resp, err := clientForIP(ip).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch audio: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body := newIdleTimeoutReader(resp.Body)

	rawProbe := make([]byte, PROBE_SIZE)
	rn, err := io.ReadFull(body, rawProbe)
	if err != nil && err != io.ErrUnexpectedEOF {
		body.Close()
		return nil, fmt.Errorf("read initial data: %w", err)
	}

	rawProbe = rawProbe[:rn]
	xingFrames := parseXingFrames(rawProbe)

	reader := &prefixedReadCloser{
		Reader: io.MultiReader(bytes.NewReader(rawProbe), body),
		closer: body,
	}

	source, err := NewMP3SourceFromReader(reader, urlStr, startTimeMs)
	if err != nil {
		return nil, err
	}

	if xingFrames > 0 && source.srcSampleRate > 0 {
		samplesPerFrame := int64(MPEG1_SAMPLES_PER_FRAME)
		if source.srcSampleRate < MPEG2_SAMPLE_RATE_THRESHOLD {
			samplesPerFrame = MPEG2_SAMPLES_PER_FRAME
		}
		source.duration = xingFrames * samplesPerFrame * 1000 / int64(source.srcSampleRate)
	} else if resp.ContentLength > 0 && source.decoder.Kbps > 0 {
		source.duration = resp.ContentLength * BITS_PER_BYTE / int64(source.decoder.Kbps)
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

var (
	tagXing = []byte("Xing")
	tagInfo = []byte("Info")
	tagVBRI = []byte("VBRI")
)

func parseXingFrames(data []byte) int64 {
	for _, tag := range [2][]byte{tagXing, tagInfo} {
		idx := bytes.Index(data, tag)
		if idx < 0 || idx+XING_DATA_OFFSET > len(data) {
			continue
		}

		flags := binary.BigEndian.Uint32(data[idx+XING_FLAGS_OFFSET:])
		offset := idx + XING_DATA_OFFSET

		if flags&1 != 0 {
			if offset+XING_FRAME_COUNT_SIZE > len(data) {
				continue
			}

			return int64(binary.BigEndian.Uint32(data[offset:]))
		}
	}

	if idx := bytes.Index(data, tagVBRI); idx >= 0 && idx+VBRI_MIN_SIZE <= len(data) {
		return int64(binary.BigEndian.Uint32(data[idx+VBRI_FRAME_COUNT_OFFSET:]))
	}

	return 0
}

func NewMP3SourceFromReader(reader io.ReadCloser, url string, startTimeMs int64) (*MP3Source, error) {
	decoder, err := minimp3.NewDecoder(reader)
	if err != nil {
		reader.Close()
		return nil, fmt.Errorf("create mp3 decoder: %w", err)
	}

	probe := make([]byte, PROBE_SIZE)
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

	resampleRatio := float64(OPUS_SAMPLE_RATE) / float64(srcSampleRate)
	inputSamplesPerChannel := int(float64(OPUS_FRAME_SIZE) / resampleRatio)

	if inputSamplesPerChannel < 1 {
		inputSamplesPerChannel = 1
	}

	inputFrameBytes := inputSamplesPerChannel * srcChannels * 2
	sampleAlign := srcChannels * 2
	inputFrameBytes = ((inputFrameBytes + sampleAlign - 1) / sampleAlign) * sampleAlign

	encoder, err := opus.NewEncoder(OPUS_SAMPLE_RATE, OPUS_CHANNELS, opus.AppAudio)
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
		inputSamples:  make([]int16, inputSamplesPerChannel*OPUS_CHANNELS),
		pcmSamples:    make([]int16, OPUS_FRAME_SIZE*OPUS_CHANNELS),
		opusBuffer:    make([]byte, OPUS_MAX_FRAME_BYTES),
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

	if s.closed.Load() || len(s.pcmBuffer) == 0 {
		return nil, io.EOF
	}

	_, err := io.ReadFull(s.pcmReader, s.pcmBuffer)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("read pcm: %w", err)
	}

	numSamplesPerChannel := len(s.pcmBuffer) / (s.srcChannels * 2)
	rawSamples := unsafe.Slice((*int16)(unsafe.Pointer(&s.pcmBuffer[0])), len(s.pcmBuffer)/2)

	if s.srcChannels == 1 {
		for i := range numSamplesPerChannel {
			sample := rawSamples[i]
			s.inputSamples[i*2] = sample
			s.inputSamples[i*2+1] = sample
		}
	} else {
		copy(s.inputSamples, rawSamples)
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

	s.position.Add(OPUS_FRAME_DURATION_MS)

	return s.opusBuffer[:numBytes], nil
}

func (s *MP3Source) resampleLinear(input, output []int16) {
	inputLen := len(input) / OPUS_CHANNELS
	outputLen := len(output) / OPUS_CHANNELS

	step := (inputLen << 16) / outputLen

	for i := range outputLen {
		fp := i * step
		srcIdx := fp >> 16
		frac := fp & 0xffff

		if srcIdx >= inputLen-1 {
			srcIdx = inputLen - 2
			frac = 0xffff
		}

		for ch := range OPUS_CHANNELS {
			idx0 := srcIdx*OPUS_CHANNELS + ch
			idx1 := idx0 + OPUS_CHANNELS

			if idx1 >= len(input) {
				idx1 = idx0
			}

			s0 := int32(input[idx0])
			s1 := int32(input[idx1])

			output[i*OPUS_CHANNELS+ch] = int16(s0 + ((s1 - s0) * int32(frac) >> 16))
		}
	}
}

func (s *MP3Source) Close() {
	if s.closed.Swap(true) {
		return
	}

	s.body.Close()

	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.decoder.Close()
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
