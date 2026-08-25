package audio

import (
	"fmt"
	"sync"

	"github.com/gen2brain/malgo"
	"github.com/hraban/opus"
	"github.com/rs/zerolog"
)

type Decoder struct {
	decoder *opus.Decoder
	logger  zerolog.Logger
}

func NewDecoder(sampleRate, channels int, logger zerolog.Logger) (*Decoder, error) {
	decoder, err := opus.NewDecoder(sampleRate, channels)
	if err != nil {
		return nil, err
	}

	return &Decoder{
		decoder: decoder,
		logger:  logger,
	}, nil
}

func (d *Decoder) DecodeS16(data []byte, out []int16) (int, error) {
	if len(data) == 0 {
		err := d.decoder.DecodePLC(out)
		if err != nil {
			return 0, err
		}
		return 960, nil
	}

	n, err := d.decoder.Decode(data, out)
	if err != nil {
		d.logger.Error().Err(err).Int("bytes", len(data)).Msg("Decode failed")
		return 0, err
	}
	return n, nil
}

func (d *Decoder) Close() error {
	return nil
}

type Encoder struct {
	encoder *opus.Encoder
	logger  zerolog.Logger
}

func NewEncoder(sampleRate, channels int, bitrate, complexity, packetLossPerc int, logger zerolog.Logger) (*Encoder, error) {
	encoder, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		return nil, err
	}

	if bitrate > 0 {
		if err := encoder.SetBitrate(bitrate); err != nil {
			return nil, err
		}
	}

	if complexity > 0 {
		if err := encoder.SetComplexity(complexity); err != nil {
			return nil, err
		}
	}

	if packetLossPerc > 0 {
		if err := encoder.SetPacketLossPerc(packetLossPerc); err != nil {
			return nil, err
		}
	}

	return &Encoder{
		encoder: encoder,
		logger:  logger,
	}, nil
}

func (e *Encoder) EncodeS16(pcm []int16, out []byte) ([]byte, error) {
	n, err := e.encoder.Encode(pcm, out)
	if err != nil {
		e.logger.Error().Err(err).Int("samples", len(pcm)).Msg("Encode failed")
		return nil, err
	}
	return out[:n], nil
}

func (e *Encoder) Close() error {
	return nil
}

type PlaybackStream struct {
	ctx           interface{}
	device        *malgo.Device
	logger        zerolog.Logger
	closed        bool
	ch            chan []byte
	mu            sync.Mutex
	frameCount    int64
	underrunCount int64
	overrunCount  int64
}

func NewPlaybackStream(
	ctx malgo.AllocatedContext,
	deviceName string,
	sampleRate int,
	channels int,
	bufferSize int,
	logger zerolog.Logger,
) (*PlaybackStream, error) {

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = uint32(channels)
	deviceConfig.SampleRate = uint32(sampleRate)
	deviceConfig.PeriodSizeInFrames = uint32(bufferSize)
	deviceConfig.Periods = 2
	deviceConfig.Alsa.NoMMap = 1

	pbStream := &PlaybackStream{
		ctx:    &ctx,
		logger: logger,
		ch:     make(chan []byte, 8),
	}

	deviceCallbacks := malgo.DeviceCallbacks{
		Data: pbStream.playbackCallback,
	}

	device, err := malgo.InitDevice(ctx.Context, deviceConfig, deviceCallbacks)
	if err != nil {
		return nil, fmt.Errorf("failed to init playback device: %w", err)
	}

	pbStream.device = device

	if err := device.Start(); err != nil {
		return nil, fmt.Errorf("failed to start playback device: %w", err)
	}

	pbStream.logger.Info().
		Str("device", deviceName).
		Int("sampleRate", sampleRate).
		Int("channels", channels).
		Int("periodFrames", bufferSize).
		Msg("Playback stream started")

	return pbStream, nil
}

func (ps *PlaybackStream) playbackCallback(pOutputSample, _ []byte, framecount uint32) {
	ps.mu.Lock()
	closed := ps.closed
	ps.frameCount++
	fc := ps.frameCount
	ps.mu.Unlock()

	if closed {
		return
	}

	needed := len(pOutputSample)

	select {
	case buf := <-ps.ch:
		n := copy(pOutputSample, buf)
		if n < needed {
			for i := n; i < needed; i++ {
				pOutputSample[i] = 0
			}
		}
	default:
		ps.mu.Lock()
		ps.underrunCount++
		ps.mu.Unlock()
		for i := range pOutputSample {
			pOutputSample[i] = 0
		}
	}

	if fc%100 == 0 {
		ps.mu.Lock()
		underruns := ps.underrunCount
		overruns := ps.overrunCount
		ps.mu.Unlock()
		ps.logger.Debug().
			Int("framecount", int(framecount)).
			Int("needBytes", needed).
			Int("chLen", len(ps.ch)).
			Int64("underruns", underruns).
			Int64("overruns", overruns).
			Msg("playback cb")
	}
}

func (ps *PlaybackStream) Play(buffer []byte) {
	ps.mu.Lock()
	closed := ps.closed
	ps.mu.Unlock()
	if closed {
		return
	}

	buf := make([]byte, len(buffer))
	copy(buf, buffer)

	select {
	case ps.ch <- buf:
	default:
		ps.mu.Lock()
		ps.overrunCount++
		ps.mu.Unlock()
		select {
		case <-ps.ch:
		default:
		}
		select {
		case ps.ch <- buf:
		default:
		}
	}
}

func (ps *PlaybackStream) Close() {
	ps.mu.Lock()
	ps.closed = true
	ps.mu.Unlock()
	if ps.device != nil {
		ps.device.Stop()
		ps.device.Uninit()
	}
	ps.mu.Lock()
	underruns := ps.underrunCount
	overruns := ps.overrunCount
	frames := ps.frameCount
	ps.mu.Unlock()
	ps.logger.Info().
		Int64("frames", frames).
		Int64("underruns", underruns).
		Int64("overruns", overruns).
		Msg("Playback stream closed")
}

type CaptureStream struct {
	ctx        interface{}
	device     *malgo.Device
	logger     zerolog.Logger
	closed     bool
	callback   func([]int16)
	mu         sync.Mutex
	frameCount int64
}

func NewCaptureStream(
	ctx malgo.AllocatedContext,
	deviceName string,
	sampleRate int,
	channels int,
	bufferSize int,
	logger zerolog.Logger,
) (*CaptureStream, error) {

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = malgo.FormatS16
	deviceConfig.Capture.Channels = uint32(channels)
	deviceConfig.SampleRate = uint32(sampleRate)
	deviceConfig.PeriodSizeInFrames = uint32(bufferSize)
	deviceConfig.Periods = 2
	deviceConfig.Alsa.NoMMap = 1

	captureStream := &CaptureStream{
		ctx:    &ctx,
		logger: logger,
	}

	deviceCallbacks := malgo.DeviceCallbacks{
		Data: captureStream.captureCallback,
	}

	device, err := malgo.InitDevice(ctx.Context, deviceConfig, deviceCallbacks)
	if err != nil {
		return nil, fmt.Errorf("failed to init capture device: %w", err)
	}

	captureStream.device = device

	if err := device.Start(); err != nil {
		return nil, fmt.Errorf("failed to start capture device: %w", err)
	}

	captureStream.logger.Info().
		Str("device", deviceName).
		Int("sampleRate", sampleRate).
		Int("channels", channels).
		Msg("Capture stream started")

	return captureStream, nil
}

func (cs *CaptureStream) captureCallback(_, pInputSamples []byte, _ uint32) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.closed {
		return
	}

	cs.frameCount++
	samples := bytesToInt16LE(pInputSamples)

	if cs.callback != nil {
		cs.callback(samples)
	}
}

func (cs *CaptureStream) StartTX(callback func([]int16)) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.callback = callback
	cs.logger.Debug().Msg("TX started - capture callback active")
}

func (cs *CaptureStream) StopTX() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.callback = nil
	cs.logger.Debug().Msg("TX stopped - capture callback cleared")
}

func (cs *CaptureStream) Close() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.closed = true
	cs.callback = nil
	if cs.device != nil {
		cs.device.Stop()
		cs.device.Uninit()
	}
	cs.logger.Info().
		Int64("frames", cs.frameCount).
		Msg("Capture stream closed")
}

func bytesToInt16LE(data []byte) []int16 {
	result := make([]int16, len(data)/2)
	for i := range result {
		result[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}
	return result
}
