package network

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kdudkov/net-ptt/internal/audio"
	"github.com/kdudkov/net-ptt/internal/rtp"
	"github.com/rs/zerolog"
	"golang.org/x/net/ipv4"
)

type JitterBuffer struct {
	slots        []jitterSlot
	maxDepth     int
	minPrebuffer int
	expected     uint16
	count        int
	mu           sync.Mutex
	lastPush     time.Time
}

type jitterSlot struct {
	seq     uint16
	payload []byte
	valid   bool
}

func NewJitterBuffer(minPrebuffer, maxDepth int) *JitterBuffer {
	slots := make([]jitterSlot, maxDepth)
	return &JitterBuffer{
		slots:        slots,
		maxDepth:     maxDepth,
		minPrebuffer: minPrebuffer,
		lastPush:     time.Now(),
	}
}

func (jb *JitterBuffer) Push(seq uint16, payload []byte) {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	if jb.expected == 0 {
		jb.expected = seq
	}

	idx := seq % uint16(jb.maxDepth)
	slot := &jb.slots[idx]

	// Exact duplicate — skip
	if slot.valid && slot.seq == seq {
		jb.lastPush = time.Now()
		return
	}

	if jb.count >= jb.maxDepth {
		return
	}

	// Overwriting a valid slot with a different seq — old packet is lost
	if slot.valid {
		jb.count--
	}

	buf := make([]byte, len(payload))
	copy(buf, payload)
	slot.seq = seq
	slot.payload = buf
	slot.valid = true

	if jb.count == 0 {
		jb.expected = seq
	}

	jb.count++
	jb.lastPush = time.Now()
}

func (jb *JitterBuffer) PopOrConceal() ([]byte, bool, bool) {
	jb.mu.Lock()
	defer jb.mu.Unlock()

	if jb.count == 0 {
		return nil, false, true
	}

	if time.Since(jb.lastPush) > 2*time.Second {
		for i := range jb.slots {
			jb.slots[i] = jitterSlot{}
		}
		jb.count = 0
		jb.expected = 0
		jb.lastPush = time.Now()
		return nil, false, true
	}

	if jb.count < jb.minPrebuffer {
		if jb.lastPush.IsZero() {
			return nil, false, true
		}
		if time.Since(jb.lastPush) < 100*time.Millisecond {
			return nil, false, true
		}
	}

	idx := jb.expected % uint16(jb.maxDepth)
	slot := &jb.slots[idx]

	if !slot.valid || slot.seq != jb.expected {
		jb.expected++
		if slot.valid {
			jb.count--
			slot.valid = false
			slot.seq = 0
			slot.payload = nil
		}
		return nil, true, false
	}

	payload := slot.payload
	slot.valid = false
	slot.seq = 0
	slot.payload = nil
	jb.count--
	jb.expected++

	return payload, false, false
}

type ReceiverPort struct {
	pc       *ipv4.PacketConn
	jitter   *JitterBuffer
	decoder  *audio.Decoder
	playback *audio.PlaybackStream
	ownSSRC  uint32

	rxCount      atomic.Int64
	parseErrors  atomic.Int64
	decodeErrors atomic.Int64
	plcCount     atomic.Int64
	lastRxTime   atomic.Int64

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	logger zerolog.Logger
}

func NewReceiverPort(iface string, multicastAddr string, port string, ownSSRC uint32, sampleRate, channels int, playback *audio.PlaybackStream, logger zerolog.Logger) (*ReceiverPort, error) {
	ifaceObj, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, fmt.Errorf("failed to get interface %s: %w", iface, err)
	}

	conn, err := net.ListenPacket("udp4", fmt.Sprintf("0.0.0.0:%s", port))
	if err != nil {
		return nil, fmt.Errorf("failed to listen multicast: %w", err)
	}

	pc := ipv4.NewPacketConn(conn)

	if err := pc.JoinGroup(ifaceObj, &net.UDPAddr{IP: net.ParseIP(multicastAddr)}); err != nil {
		return nil, err
	}

	decoder, err := audio.NewDecoder(sampleRate, channels, logger)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("failed to create decoder: %w", err)
	}

	logger.Info().
		Str("interface", iface).
		Str("multicast_addr", multicastAddr).
		Str("port", port).
		Msg("Listening on multicast group")

	return &ReceiverPort{
		pc:       pc,
		jitter:   NewJitterBuffer(5, 24),
		decoder:  decoder,
		playback: playback,
		ownSSRC:  ownSSRC,
		logger:   logger,
	}, nil
}

func (rp *ReceiverPort) Start(ctx context.Context) error {
	rp.ctx, rp.cancel = context.WithCancel(ctx)
	rp.wg.Add(2)
	go rp.receiveLoop()
	go rp.playoutLoop()
	return nil
}

func (rp *ReceiverPort) receiveLoop() {
	defer rp.wg.Done()
	buf := make([]byte, 4096)

	for {
		select {
		case <-rp.ctx.Done():
			return
		default:
			rp.pc.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, _, _, err := rp.pc.ReadFrom(buf)
			if err != nil {
				continue
			}

			rp.rxCount.Add(1)
			rp.lastRxTime.Store(time.Now().UnixNano())

			packet, err := rtp.ParsePacket(buf[:n])
			if err != nil {
				rp.parseErrors.Add(1)
				rp.logger.Debug().Err(err).Msg("Failed to parse RTP packet")
				continue
			}

			if packet.Header.SSRC == rp.ownSSRC {
				continue
			}

			rxCount := rp.rxCount.Load()
			if rxCount <= 10 || rxCount%50 == 0 {
				rp.logger.Debug().
					Int64("rxCount", rxCount).
					Uint16("seq", packet.Header.Sequence).
					Int("payloadLen", len(packet.Payload)).
					Msg("recv")
			}

			rp.jitter.Push(packet.Header.Sequence, packet.Payload)
		}
	}
}

func (rp *ReceiverPort) playoutLoop() {
	defer rp.wg.Done()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-rp.ctx.Done():
			return
		case <-ticker.C:
			payload, conceal, idle := rp.jitter.PopOrConceal()
			if idle {
				continue
			}

			out := make([]int16, 960)

			if payload != nil {
				_, err := rp.decoder.DecodeS16(payload, out)
				if err != nil {
					rp.decodeErrors.Add(1)
					rp.logger.Warn().Err(err).Int("payloadLen", len(payload)).Msg("decode error")
					continue
				}
			} else if conceal {
				_, err := rp.decoder.DecodeS16(nil, out)
				if err != nil {
					rp.decodeErrors.Add(1)
					rp.logger.Warn().Err(err).Msg("plc error")
					continue
				}
				rp.plcCount.Add(1)
			}

			rp.playback.Play(int16ToBytes(out))
		}
	}
}

func int16ToBytes(data []int16) []byte {
	out := make([]byte, len(data)*2)
	for i, v := range data {
		out[i*2] = byte(v)
		out[i*2+1] = byte(v >> 8)
	}
	return out
}

func (rp *ReceiverPort) GetStats() (rxCount int64, parseErrors int64, decodeErrors int64, plcCount int64) {
	return rp.rxCount.Load(), rp.parseErrors.Load(), rp.decodeErrors.Load(), rp.plcCount.Load()
}

func (rp *ReceiverPort) IsReceiving() bool {
	t := rp.lastRxTime.Load()
	if t == 0 {
		return false
	}
	return time.Since(time.Unix(0, t)) < 500*time.Millisecond
}

func (rp *ReceiverPort) Close() error {
	if rp.cancel != nil {
		rp.cancel()
	}
	rp.wg.Wait()
	if rp.decoder != nil {
		rp.decoder.Close()
	}
	if rp.pc != nil {
		return rp.pc.Close()
	}
	return nil
}
