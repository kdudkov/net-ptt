package client

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/kdudkov/net-ptt/internal/audio"
	"github.com/kdudkov/net-ptt/internal/config"
	"github.com/kdudkov/net-ptt/internal/network"
	"github.com/rs/zerolog"
)

type CommsClient struct {
	config *config.ClientConfig
	logger zerolog.Logger

	malgoCtx *malgo.AllocatedContext

	encoder  *audio.Encoder
	playback *audio.PlaybackStream
	capture  *audio.CaptureStream

	recvPort *network.ReceiverPort
	txPort   *network.TransmitterPort

	ssrc uint32

	startTime time.Time

	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	shutdown bool
	mu       sync.RWMutex
}

func NewClient(cfg *config.ClientConfig, logger zerolog.Logger) (*CommsClient, error) {
	if err := config.Validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	config.ApplyDefaults(cfg)

	client := &CommsClient{
		config:    cfg,
		logger:    logger,
		startTime: time.Now(),
		ssrc:      rand.Uint32() | 1,
	}

	if err := client.initAudio(); err != nil {
		return nil, fmt.Errorf("failed to initialize audio: %w", err)
	}

	if err := client.initNetwork(); err != nil {
		return nil, fmt.Errorf("failed to initialize network: %w", err)
	}

	return client, nil
}

func (c *CommsClient) initAudio() error {
	var err error
	malgoCtx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return fmt.Errorf("failed to initialize malgo: %w", err)
	}
	c.malgoCtx = malgoCtx

	c.encoder, err = audio.NewEncoder(
		c.config.SampleRate,
		c.config.Channels,
		c.config.Bitrate,
		c.config.Complexity,
		c.config.PacketLossPerc,
		c.logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create encoder: %w", err)
	}

	c.playback, err = audio.NewPlaybackStream(
		*c.malgoCtx,
		c.config.OutputDevice,
		c.config.SampleRate,
		c.config.Channels,
		960,
		c.logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create playback stream: %w", err)
	}

	c.capture, err = audio.NewCaptureStream(
		*c.malgoCtx,
		c.config.InputDevice,
		c.config.SampleRate,
		c.config.Channels,
		960,
		c.logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create capture stream: %w", err)
	}

	return nil
}

func (c *CommsClient) initNetwork() error {
	recvPort, txPort, err := c.buildPorts(c.config.Channel)
	if err != nil {
		return err
	}
	c.recvPort = recvPort
	c.txPort = txPort
	return nil
}

// buildPorts creates a fresh receiver/transmitter pair bound to the given channel.
func (c *CommsClient) buildPorts(channel int) (*network.ReceiverPort, *network.TransmitterPort, error) {
	port, err := config.TalkGroupPort(channel)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get talk group port: %w", err)
	}

	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(c.config.MulticastAddr, port))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve addr: %w", err)
	}

	recvPort, err := network.NewReceiverPort(
		c.config.Interface,
		c.config.MulticastAddr,
		port,
		c.ssrc,
		c.config.SampleRate,
		c.config.Channels,
		c.playback,
		c.logger,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create receiver port: %w", err)
	}

	txPort, err := network.NewTransmitterPort(
		c.config.Interface,
		addr,
		c.ssrc,
		c.encoder,
		c.capture,
		c.logger,
	)
	if err != nil {
		recvPort.Close()
		return nil, nil, fmt.Errorf("failed to create transmitter port: %w", err)
	}

	return recvPort, txPort, nil
}

// SwitchChannel tears down the current receiver/transmitter and rebinds them
// to the given talk group channel. Any in-progress transmission is stopped.
func (c *CommsClient) SwitchChannel(channel int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.shutdown {
		return fmt.Errorf("client is shut down")
	}

	recvPort, txPort, err := c.buildPorts(channel)
	if err != nil {
		return err
	}

	oldRecv, oldTx := c.recvPort, c.txPort
	if oldTx != nil {
		oldTx.SetTxEnabled(false)
	}

	// Stop old ports before starting new ones to avoid goroutine leaks
	// and concurrent decoder access
	if oldRecv != nil {
		oldRecv.Close()
	}
	if oldTx != nil {
		oldTx.Close()
	}

	if err := recvPort.Start(c.ctx); err != nil {
		recvPort.Close()
		txPort.Close()
		return fmt.Errorf("failed to start receiver: %w", err)
	}
	if err := txPort.Start(c.ctx); err != nil {
		recvPort.Close()
		txPort.Close()
		return fmt.Errorf("failed to start transmitter: %w", err)
	}

	c.recvPort = recvPort
	c.txPort = txPort
	c.config.Channel = channel

	c.logger.Info().Int("channel", channel).Msg("Switched channel")
	return nil
}

// StartTransmit enables the microphone and begins sending audio on the
// currently selected channel.
func (c *CommsClient) StartTransmit() {
	c.mu.RLock()
	txPort := c.txPort
	c.mu.RUnlock()
	if txPort != nil && !txPort.IsTxEnabled() {
		txPort.SetTxEnabled(true)
	}
}

// StopTransmit disables the microphone.
func (c *CommsClient) StopTransmit() {
	c.mu.RLock()
	txPort := c.txPort
	c.mu.RUnlock()
	if txPort != nil && txPort.IsTxEnabled() {
		txPort.SetTxEnabled(false)
	}
}

// IsTransmitting reports whether the client is currently transmitting.
func (c *CommsClient) IsTransmitting() bool {
	c.mu.RLock()
	txPort := c.txPort
	c.mu.RUnlock()
	return txPort != nil && txPort.IsTxEnabled()
}

func (c *CommsClient) IsReceiving() bool {
	c.mu.RLock()
	recvPort := c.recvPort
	c.mu.RUnlock()
	if recvPort == nil {
		return false
	}
	return recvPort.IsReceiving()
}

func (c *CommsClient) CurrentChannel() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config.Channel
}

func (c *CommsClient) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.shutdown {
		return fmt.Errorf("client is shut down")
	}

	c.ctx, c.cancel = context.WithCancel(context.Background())

	c.logger.Info().
		Int("channel", c.config.Channel).
		Str("interface", c.config.Interface).
		Str("multicast_addr", c.config.MulticastAddr).
		Msg("Starting comms client")

	if err := c.recvPort.Start(c.ctx); err != nil {
		return fmt.Errorf("failed to start receiver: %w", err)
	}

	if err := c.txPort.Start(c.ctx); err != nil {
		return fmt.Errorf("failed to start transmitter: %w", err)
	}

	c.wg.Add(1)
	go c.runStats()

	c.logger.Info().Msg("Comms client started successfully")
	return nil
}

func (c *CommsClient) runStats() {
	defer c.wg.Done()

	if c.config.StatsInterval <= 0 {
		return
	}

	ticker := time.NewTicker(time.Duration(c.config.StatsInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.mu.RLock()
			recvPort := c.recvPort
			txPort := c.txPort
			c.mu.RUnlock()
			if recvPort == nil || txPort == nil {
				continue
			}
			rxCnt, parseErrs, decErrs, plcCnt := recvPort.GetStats()
			txCnt, encErrs, dropped := txPort.GetStats()
			duration := time.Since(c.startTime).Seconds()

			c.logger.Info().
				Dur("uptime", time.Since(c.startTime)).
				Int64("rx_total", rxCnt).
				Float64("rx_per_sec", float64(rxCnt)/duration).
				Int64("tx_total", txCnt).
				Float64("tx_per_sec", float64(txCnt)/duration).
				Int64("parse_errors", parseErrs).
				Int64("decode_errors", decErrs).
				Int64("encode_errors", encErrs).
				Int64("plc_count", plcCnt).
				Int64("dropped_frames", dropped).
				Msg("Statistics")
		}
	}
}

func (c *CommsClient) Shutdown() {
	c.mu.Lock()
	if c.shutdown {
		c.mu.Unlock()
		return
	}
	c.shutdown = true
	c.mu.Unlock()

	c.logger.Info().Msg("Shutting down comms client")

	if c.cancel != nil {
		c.cancel()
	}

	c.wg.Wait()

	if c.recvPort != nil {
		c.recvPort.Close()
	}

	if c.txPort != nil {
		c.txPort.Close()
	}

	if c.capture != nil {
		c.capture.Close()
	}

	if c.playback != nil {
		c.playback.Close()
	}

	if c.encoder != nil {
		c.encoder.Close()
	}

	rxCnt, parseErrs, decErrs, plcCnt := c.recvPort.GetStats()
	txCnt, encErrs, dropped := c.txPort.GetStats()

	c.logger.Info().
		Dur("uptime", time.Since(c.startTime)).
		Int64("rx_total_frames", rxCnt).
		Int64("tx_total_frames", txCnt).
		Int64("rx_parse_errors", parseErrs).
		Int64("rx_decode_errors", decErrs).
		Int64("tx_encode_errors", encErrs).
		Int64("rx_plc_count", plcCnt).
		Int64("tx_dropped_frames", dropped).
		Msg("Final statistics - Comms client shut down")
}


