# Network PTT

A **standalone PTT client** for voice communications that uses  malgo + opus for audio processing.

## **Stack**

This client is built with the same proven components:
- **malgo**: Audio I/O library
- **libopus**: Opus codec via hraban/opus bindings
- **RTP protocol**: Same multicast implementation
- **PTT control**: Interactive terminal UI (bubbletea) - arrow keys to pick a channel, space to talk
- **Cross-platform**: Builds for Linux amd64/arm64

## Features

- **Libraries**: malgo + libopus for audio processing
- **PTT functionality**: Both receive and transmit voice
- **Opus codec**: 48kHz mono with configurable bitrate (default 32kbps)
- **Multicast support**: Join/leave multicast groups on specified interface
- **RTP protocol**: Standard RTP over UDP with proper jitter buffering
- **PTT control**: Terminal UI channel picker with hold-to-talk on space
- **Statistics**: Detailed logging of network and audio metrics
- **Cross-platform**: Builds for Linux amd64, arm64 and OSX

## Compatibility with openmanetd

This client is intended to be compatible with openmanetd voice communication:
- Same multicast addresses and ports
- Same RTP packet format
- Same Opus codec configuration
- Same jitter buffer behavior

## Installation

### Lite Version (No CGO - Portable)

```bash
cd ../ptt
task build-lite
```

This produces portable binaries for:
- `bin/net-ptt-lite` (Linux x86_64) - No CGO
- `bin/net-ptt-arm64-lite` (Linux ARM64) - No CGO

These work on any Linux system without requiring PortAudio/libopus installation.

### Full Version (With CGO - Real Audio)

Build on Linux for real audio processing:

```bash
sudo apt install portaudio19-dev libopus-dev  # Ubuntu/Debian
task build                            # On the target Linux system
```

This produces:
- `bin/net-ptt` - Full audio support

## Usage

### Basic usage

```bash
# Launch the channel picker with default devices
./net-ptt

# Launch with specific audio devices
./net-ptt \
  --output-device "alsa_output.pci-0000_00_1f.3.analog-stereo" \
  --input-device "alsa_input.pci-0000_00_1f.3.analog-stereo" \
  --interface br-ahwlan
```

Once running, use the arrow keys (or `j`/`k`) to select a channel, hold
`space` to talk, and press `q` or `ctrl+c` to quit.

### List available audio devices

```bash
./net-ptt --list-devices
```

### Debug mode with statistics

```bash
./net-ptt --log-level DEBUG --stats-interval 30
```

## Command-line options

### Required options

- `--output-device <name>`: Audio output device name or index
- `--input-device <name>`: Audio input device name or index

### Optional options

**Network:**
- `--interface <name>`: Network interface for multicast (default: `br-ahwlan`)
- `--multicast-addr <addr>`: Multicast address (default: `239.192.41.1`)

**Audio:**
- `--sample-rate <hz>`: Sample rate (default: `48000`)
- `--channels <num>`: Number of channels (default: `1` - mono)
- `--complexity <1-10>`: Opus encoder complexity (default: `5`)
- `--bitrate <bps>`: Target bitrate (default: `32000`)

**Jitter buffer:**
- `--jitter-depth <num>`: Jitter buffer depth (default: `24`)
- `--min-latency <ms>`: Minimum latency in milliseconds (default: `100`)

**Logging and statistics:**
- `--log-level <level>`: Log level - DEBUG, INFO, WARN, ERROR (default: `INFO`)
  Logs are written to `net-ptt.log` in the current directory (the
  terminal is used by the TUI).
- `--stats-interval <seconds>`: Statistics interval (default: `0` - disabled)

**Other:**
- `--loopback`: Receive own packets (default: `false`)
- `--list-devices`: List available audio devices and exit
- `--version`: Show version information

## Audio device selection

You can specify audio devices by:
1. **Index**: `0`, `1`, `2`, etc.
2. **Name substring**: `"alsa_output"`, `"USB"`, `"headset"`, etc.
3. **Full name**: Exact match from `--list-devices` output

## PTT operation

PTT is controlled from the terminal UI:

1. Use `up`/`down` (or `j`/`k`) to select a channel from the list; the
   receiver/transmitter switch to it immediately.
2. Hold `space` to transmit, release it to stop.
3. If your terminal doesn't support the Kitty keyboard protocol (needed to
   detect key release), the UI falls back to toggle mode: press `space` once
   to start talking, press it again to stop.
4. Press `q` or `ctrl+c` to quit.

## Channel mapping

The TUI lists 8 channels, mapped to UDP ports as follows:
- Channel 1 → Port 38801
- Channel 2 → Port 38803
- Channel 3 → Port 38805
- ...
- Channel N → Port 38801 + (N-1) * 2

All channels use the same multicast address (239.192.41.1).

## Technical details

### Protocol

- **Codec**: Opus (RFC 6716)
- **Sample rate**: 48 kHz
- **Frame size**: 960 samples (20ms)
- **Payload type**: 111 (dynamic)
- **Transport**: RTP over UDP multicast

### Jitter buffer

- **Depth**: 24 packets (480ms max)
- **Prebuffer**: 5 packets (100ms min latency)
- **Packet loss concealment**: Opus PLC
- **SSRC tracking**: Automatic talker change handling

### Performance considerations

- Capture/playback period: 20ms
- Encode/decode time: < 10ms
- One-way latency: ~120-180ms (depending on jitter)
- CPU usage: < 5% on arm64 at 32kbps

## Examples

See `examples/usage.sh` for more usage examples.

## Troubleshooting

### No audio devices found

Run `--list-devices` to see available devices:

```bash
./net-ptt --list-devices
```

## Known limitations

- Only one active channel at a time (switching stops the previous one)
- No recording functionality
- No web interface
- CGO required (depends on libopus and miniaudio)

## Future enhancements

- Multi-channel playback
- Audio recording
- TCP fallback
- Windows support
- Web interface ?

## License

[Add your license here]
