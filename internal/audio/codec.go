//go:build !cgo

package audio

import (
	"sync"

	"github.com/rs/zerolog"
)

type PlaybackStream struct {
	logger zerolog.Logger
	mu     sync.Mutex
	ch     chan []byte
	closed bool
}

type CaptureStream struct {
	logger   zerolog.Logger
	mu       sync.Mutex
	callback func([]int16)
	closed   bool
}

type Decoder struct {
	logger zerolog.Logger
	mu     sync.Mutex
}

type Encoder struct {
	logger zerolog.Logger
	mu     sync.Mutex
}

func NewDecoder(sampleRate, channels int, logger zerolog.Logger) (*Decoder, error) {
	return &Decoder{logger: logger}, nil
}

func NewEncoder(sampleRate, channels int, bitrate, complexity, packetLossPerc int, logger zerolog.Logger) (*Encoder, error) {
	return &Encoder{logger: logger}, nil
}

func (d *Decoder) DecodeS16(data []byte, out []int16) (int, error) {
	if data == nil {
		for i := range out {
			out[i] = 0
		}
		return 960, nil
	}
	samples := len(data) / 2
	for i := 0; i < samples && i < len(out); i++ {
		out[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}
	for i := samples; i < len(out); i++ {
		out[i] = 0
	}
	return 960, nil
}

func (d *Decoder) Close() error { return nil }

func (e *Encoder) EncodeS16(pcm []int16, _ []byte) ([]byte, error) {
	result := make([]byte, len(pcm)*2)
	for i, v := range pcm {
		result[i*2] = byte(v)
		result[i*2+1] = byte(v >> 8)
	}
	return result, nil
}

func (e *Encoder) Close() error { return nil }

func NewPlaybackStream(_ interface{}, deviceName string, sampleRate, channels, bufferSize int, logger zerolog.Logger) (*PlaybackStream, error) {
	logger.Debug().Str("device", deviceName).Int("sampleRate", sampleRate).Int("channels", channels).Msg("Creating dummy playback stream")
	return &PlaybackStream{
		logger: logger,
		ch:     make(chan []byte, 8),
	}, nil
}

func (ps *PlaybackStream) Play(buffer []byte) {
	select {
	case ps.ch <- buffer:
	default:
	}
}

func (ps *PlaybackStream) Close() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.closed = true
}

func NewCaptureStream(_ interface{}, deviceName string, sampleRate, channels, bufferSize int, logger zerolog.Logger) (*CaptureStream, error) {
	logger.Debug().Str("device", deviceName).Int("sampleRate", sampleRate).Int("channels", channels).Msg("Creating dummy capture stream")
	return &CaptureStream{logger: logger}, nil
}

func (cs *CaptureStream) StartTX(callback func([]int16)) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.callback = callback
}

func (cs *CaptureStream) StopTX() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.callback = nil
}

func (cs *CaptureStream) Close() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.closed = true
	cs.callback = nil
}

func bytesToInt16LE(data []byte) []int16 {
	result := make([]int16, len(data)/2)
	for i := range result {
		result[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}
	return result
}
