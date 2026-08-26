package network

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"

	"github.com/kdudkov/net-ptt/internal/audio"
	"github.com/kdudkov/net-ptt/internal/rtp"
	"github.com/rs/zerolog"
)

type TransmitterPort struct {
	conn     *net.UDPConn
	encoder  *audio.Encoder
	capture  *audio.CaptureStream

	ssrc          uint32
	seqNum        atomic.Uint32
	timestamp     uint32
	txEnabled     atomic.Bool
	talkspurt     bool
	txCount       atomic.Int64
	encodeErrors  atomic.Int64
	droppedFrames atomic.Int64

	logger zerolog.Logger
}

func NewTransmitterPort(iface string, multicastAddr *net.UDPAddr, ssrc uint32, encoder *audio.Encoder, capture *audio.CaptureStream, logger zerolog.Logger) (*TransmitterPort, error) {
	conn, err := net.DialUDP("udp", nil, multicastAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect UDP: %w", err)
	}

	tp := &TransmitterPort{
		conn:      conn,
		encoder:   encoder,
		capture:   capture,
		ssrc:      ssrc,
		txEnabled: atomic.Bool{},
		talkspurt: true,
		logger:    logger,
	}

	logger.Info().Uint32("ssrc", ssrc).Msg("Transmitter created")
	return tp, nil
}

func (tp *TransmitterPort) processFrame(pcm []int16) {
	if !tp.txEnabled.Load() {
		return
	}

	out := make([]byte, 400)
	payload, err := tp.encoder.EncodeS16(pcm, out)
	if err != nil {
		tp.encodeErrors.Add(1)
		tp.logger.Debug().Err(err).Msg("Failed to encode audio")
		return
	}

	seq := uint16(tp.seqNum.Add(1) - 1)
	marker := tp.talkspurt
	tp.talkspurt = false

	header := rtp.BuildRTPHeader(seq, tp.timestamp, tp.ssrc, rtp.OpusPayloadType, marker)
	packet := append(header, payload...)

	if _, err := tp.conn.Write(packet); err != nil {
		tp.logger.Debug().Err(err).Msg("Failed to send RTP packet")
		tp.droppedFrames.Add(1)
		return
	}

	tp.timestamp += uint32(len(pcm))
	tp.txCount.Add(1)
}

func (tp *TransmitterPort) Start(ctx context.Context) error {
	tp.capture.StartTX(tp.processFrame)

	return nil
}

func (tp *TransmitterPort) SetTxEnabled(enabled bool) {
	tp.txEnabled.Store(enabled)
	if enabled {
		tp.talkspurt = true
		tp.capture.StartTX(tp.processFrame)
	} else {
		tp.capture.StopTX()
	}
}

func (tp *TransmitterPort) IsTxEnabled() bool {
	return tp.txEnabled.Load()
}

func (tp *TransmitterPort) GetStats() (txCount int64, encodeErrors int64, droppedFrames int64) {
	return tp.txCount.Load(), tp.encodeErrors.Load(), tp.droppedFrames.Load()
}

func (tp *TransmitterPort) Close() error {
	if tp.conn != nil {
		return tp.conn.Close()
	}
	return nil
}
