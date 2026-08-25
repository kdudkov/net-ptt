# Building on Linux with CGO

## Prerequisites

### Ubuntu/Debian:
```bash
sudo apt update
sudo apt install portaudio19-dev libopus-dev build-essential
```

### Fedora/RHEL:
```bash
sudo dnf install portaudio-devel opus-devel gcc golang
```

### Arch Linux:
```bash
sudo pacman -S portaudio opus gcc go
```

## Building
Перейдите в директорию проекта и выполните:

```bash
cd /path/to/ptt
task build
```

Это создаст `bin/net-ptt` с полной поддержкой реального аудио.

## Testing Audio Devices

List available devices:
```bash
./bin/net-ptt --list-devices
```

## Running the Client

Basic usage with default devices:
```bash
./bin/net-ptt --output-device default --input-device default
```

Specific devices:
```bash
./bin/net-ptt \
  --output-device 0 \
  --input-device 1
```

This launches a terminal UI: use up/down to pick a channel and hold space to talk.

## Troubleshooting

### CGO Errors:

If you see CGO compilation errors, ensure:
1. PortAudio is installed: `dpkg -l | grep portaudio`
2. Opus is installed: `dpkg -l | grep opus`
3. CGO is enabled: `export CGO_ENABLED=1`

### Device Not Found:

Use `--list-devices` to see available devices and pick correct index or name.

### No Audio:

Check permissions and that the audio device is not in use by another application.

