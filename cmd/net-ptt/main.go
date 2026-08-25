package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/rs/zerolog"

	"github.com/kdudkov/net-ptt/internal/client"
	"github.com/kdudkov/net-ptt/internal/config"
	"github.com/kdudkov/net-ptt/internal/tui"
)

var (
	flagInterface     = flag.String("interface", "en0", "Network interface for multicast")
	flagMulticastAddr = flag.String("multicast-addr", "239.192.41.1", "Multicast address")

	flagInputDevice  = flag.String("input-device", "0", "Audio input device name or index [REQUIRED]")
	flagOutputDevice = flag.String("output-device", "0", "Audio output device name or index [REQUIRED]")

	flagSampleRate = flag.Int("sample-rate", 48000, "Sample rate in Hz")
	flagChannels   = flag.Int("channels", 1, "Number of audio channels")
	flagComplexity = flag.Int("complexity", 5, "Opus encoder complexity (0-10)")
	flagBitrate    = flag.Int("bitrate", 32000, "Target bitrate in bps")

	flagJitterDepth = flag.Int("jitter-depth", 24, "Jitter buffer depth")
	flagMinLatency  = flag.Int("min-latency", 100, "Minimum latency in milliseconds")

	flagLogLevel      = flag.String("log-level", "INFO", "Log level (DEBUG, INFO, WARN, ERROR)")
	flagStatsInterval = flag.Int("stats-interval", 0, "Statistics reporting interval in seconds (0 = disabled)")

	flagListDevices = flag.Bool("list-devices", false, "List available audio devices and exit")
	flagVersion     = flag.Bool("version", false, "Show version information")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Comms Client v%s\n\n", version)
		fmt.Fprintf(os.Stderr, "Simple standalone PTT client for voice communications.\n")
		fmt.Fprintf(os.Stderr, "Libraries used: malgo + opus\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Required options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *flagVersion {
		fmt.Printf("net-ptt version %s\n", getVersionFull())
		fmt.Println("Standalone voice PTT client")
		os.Exit(0)
	}

	if *flagListDevices {
		listMalgoDevices()
		os.Exit(0)
	}

	if *flagInputDevice == "" {
		fmt.Fprintf(os.Stderr, "Error: --input-device is required\n")
		flag.Usage()
		os.Exit(1)
	}

	if *flagOutputDevice == "" {
		fmt.Fprintf(os.Stderr, "Error: --output-device is required\n")
		flag.Usage()
		os.Exit(1)
	}

	switch *flagLogLevel {
	case "DEBUG":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "INFO":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "WARN":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "ERROR":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// Logs go to a file since the TUI takes over stdout/stderr rendering.
	logFile, err := os.OpenFile("net-ptt.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to open log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	logger := zerolog.New(logFile).With().Timestamp().Logger()

	var commsClient *client.CommsClient

	channels := config.Channels()
	initialChannel := channels[0].Number

	logger.Info().
		Str("version", version).
		Int("channel", initialChannel).
		Str("interface", *flagInterface).
		Str("multicast_addr", *flagMulticastAddr).
		Str("input_device", *flagInputDevice).
		Str("output_device", *flagOutputDevice).
		Msg("Starting comms client")

	cfg := config.ClientConfig{
		Channel:       initialChannel,
		Interface:     *flagInterface,
		MulticastAddr: *flagMulticastAddr,

		InputDevice:  *flagInputDevice,
		OutputDevice: *flagOutputDevice,

		SampleRate:     *flagSampleRate,
		Channels:       *flagChannels,
		Complexity:     *flagComplexity,
		Bitrate:        *flagBitrate,
		PacketLossPerc: 20,

		JitterDepth: *flagJitterDepth,
		MinLatency:  *flagMinLatency,

		LogLevel:      *flagLogLevel,
		StatsInterval: *flagStatsInterval,
	}

	commsClient, err = client.NewClient(&cfg, logger)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create comms client")
		os.Exit(1)
	}

	if err := commsClient.Start(); err != nil {
		logger.Error().Err(err).Msg("Failed to start comms client")
		os.Exit(1)
	}

	model := tui.New(commsClient, channels, initialChannel)
	if _, err := tea.NewProgram(model).Run(); err != nil {
		logger.Error().Err(err).Msg("TUI exited with error")
	}

	commsClient.Shutdown()
	logger.Info().Msg("Shutdown complete")
}
