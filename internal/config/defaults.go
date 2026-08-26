package config

import (
	"fmt"
	"net"
	"strconv"
)

const (
	DefaultMulticastAddr = "239.192.41.1"
	DefaultInterface     = "br-ahwlan"
	DefaultTalkGroupPort = 38801
	TalkGroupPortStride  = 2
	TalkGroupMaxChannel  = 32

	// TUIChannelCount is the number of channels listed in the channel picker.
	TUIChannelCount = 8

	DefaultSampleRate      = 48000
	DefaultChannels        = 1
	DefaultComplexity      = 5
	DefaultBitrate         = 32000
	DefaultPacketLossPerc  = 20

	DefaultJitterDepth = 24
	DefaultMinLatency  = 100

	DefaultLogLevel = "INFO"
)

type ClientConfig struct {
	Channel       int
	Interface     string
	MulticastAddr string

	InputDevice  string
	OutputDevice string

	SampleRate     int
	Channels       int
	Complexity     int
	Bitrate        int
	PacketLossPerc int

	JitterDepth int
	MinLatency  int

	LogLevel      string
	StatsInterval int
}

func Validate(cfg *ClientConfig) error {
	if cfg.Channel < 1 || cfg.Channel > TalkGroupMaxChannel {
		return fmt.Errorf("channel must be between 1 and %d", TalkGroupMaxChannel)
	}

	if cfg.MulticastAddr != "" {
		ip := net.ParseIP(cfg.MulticastAddr)
		if ip == nil || !ip.IsMulticast() {
			return fmt.Errorf("invalid multicast address: %s", cfg.MulticastAddr)
		}
	}

	if cfg.InputDevice == "" {
		return fmt.Errorf("input device must be specified")
	}

	if cfg.OutputDevice == "" {
		return fmt.Errorf("output device must be specified")
	}

	if cfg.SampleRate != 48000 && cfg.SampleRate != 24000 && cfg.SampleRate != 16000 {
		return fmt.Errorf("sample rate must be 48000, 24000, or 16000 (got %d)", cfg.SampleRate)
	}

	if cfg.Channels != 1 {
		return fmt.Errorf("only mono audio is supported (got %d channels)", cfg.Channels)
	}

	if cfg.Complexity < 0 || cfg.Complexity > 10 {
		return fmt.Errorf("complexity must be between 0 and 10 (got %d)", cfg.Complexity)
	}

	if cfg.Bitrate < 6000 || cfg.Bitrate > 510000 {
		return fmt.Errorf("bitrate must be between 6000 and 510000 bps (got %d)", cfg.Bitrate)
	}

	if cfg.JitterDepth < 12 || cfg.JitterDepth > 48 {
		return fmt.Errorf("jitter depth must be between 12 and 48 (got %d)", cfg.JitterDepth)
	}

	if cfg.MinLatency < 50 || cfg.MinLatency > 500 {
		return fmt.Errorf("min latency must be between 50 and 500 ms (got %d)", cfg.MinLatency)
	}

	switch cfg.LogLevel {
	case "DEBUG", "INFO", "WARN", "ERROR":
	default:
		return fmt.Errorf("invalid log level: %s (must be DEBUG, INFO, WARN, or ERROR)", cfg.LogLevel)
	}

	return nil
}

func TalkGroupPort(channel int) (string, error) {
	if channel < 1 || channel > TalkGroupMaxChannel {
		return "0", fmt.Errorf("talk group channel %d out of range [1, %d]", channel, TalkGroupMaxChannel)
	}

	return strconv.Itoa(DefaultTalkGroupPort + (channel-1)*TalkGroupPortStride), nil
}

func ApplyDefaults(cfg *ClientConfig) {
	if cfg.MulticastAddr == "" {
		cfg.MulticastAddr = DefaultMulticastAddr
	}

	if cfg.Interface == "" {
		if name := defaultInterfaceName(); name != "" {
			cfg.Interface = name
		} else {
			cfg.Interface = DefaultInterface
		}
	}

	if cfg.SampleRate == 0 {
		cfg.SampleRate = DefaultSampleRate
	}

	if cfg.Channels == 0 {
		cfg.Channels = DefaultChannels
	}

	if cfg.Complexity == 0 {
		cfg.Complexity = DefaultComplexity
	}

	if cfg.Bitrate == 0 {
		cfg.Bitrate = DefaultBitrate
	}

	if cfg.PacketLossPerc == 0 {
		cfg.PacketLossPerc = DefaultPacketLossPerc
	}

	if cfg.JitterDepth == 0 {
		cfg.JitterDepth = DefaultJitterDepth
	}

	if cfg.MinLatency == 0 {
		cfg.MinLatency = DefaultMinLatency
	}

	if cfg.LogLevel == "" {
		cfg.LogLevel = DefaultLogLevel
	}
}

// Channel describes a selectable talk group channel.
type Channel struct {
	Number int
	Name   string
	Port   int
}

// Channels returns the list of channels shown in the channel picker.
func Channels() []Channel {
	channels := make([]Channel, 0, TUIChannelCount)
	for n := 1; n <= TUIChannelCount; n++ {
		channels = append(channels, Channel{
			Number: n,
			Name:   fmt.Sprintf("Channel %d", n),
			Port:   DefaultTalkGroupPort + (n-1)*TalkGroupPortStride,
		})
	}
	return channels
}

// defaultInterfaceName finds the network interface that the kernel's
// default route points at. It works by doing a UDP "dial" to a public
// address (no packets are actually sent for UDP) and looking up which
// interface owns the source IP the kernel chose.
func defaultInterfaceName() string {
	conn, err := net.Dial("udp", "8.8.8.8:53")
	if err != nil {
		return ""
	}
	defer conn.Close()

	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return ""
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipnet.IP.Equal(localAddr.IP) {
					return iface.Name
				}
			}
		}
	}
	return ""
}
