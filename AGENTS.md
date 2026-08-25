# AGENTS.md

Go module `github.com/kdudkov/net-ptt` (Go 1.25) — standalone PTT/RTP voice client (bubbletea TUI + malgo/opus audio), inspired by (and protocol-compatible with) `openmanetd`.

## Build (use `task`, not raw `go build`)

- `task build-macos` — builds on macOS with CGO using Homebrew opus/opusfile/libogg. This is the **only build that currently works out of the box** on this dev machine. Requires `brew install opus opusfile libogg` first.
- `task build-amd64` / `task build-arm64` — Linux CGO builds; require `portaudio19-dev libopus-dev` (Debian) installed on the **target** system (no cross-compile toolchain configured).
- `task build-lite` — **currently broken**: builds with `CGO_ENABLED=0 -tags=!cgo`, but the `github.com/hraban/opus` dependency itself has no non-CGO stub (only this repo's own `internal/audio/codec.go` has a `//go:build !cgo` variant). Fails with `undefined: Stream` in `streams_map.go`. Don't waste time debugging the app code for this — the fix would have to live in the opus dependency or by vendoring a stub.
- `task deps` — `go mod download && go mod tidy`.
- `task clean` — removes `bin/`.

There is no cross-compiling from macOS to Linux with CGO enabled — Linux binaries must be built on Linux (or via `task build-lite`, once fixed).

## Test / Lint

- `task test` runs `go test -v ./...` — **there are currently no test files anywhere in the repo**; this is a no-op smoke check, not real coverage.
- `task lint` runs `golangci-lint run` if installed (no `.golangci.yml` config present, so defaults apply).
- No CI workflows exist in this repo.

## Architecture

- `cmd/net-ptt/main.go` — CLI flags + wiring, always compiled.
- `cmd/net-ptt/main_cgo.go` — `//go:build cgo` — device listing via malgo; the `!cgo` build of `main` has no `listMalgoDevices` equivalent (this is one reason `build-lite` is not truly usable even after the opus issue is fixed).
- `internal/audio` — `device.go` (CGO malgo/opus device I/O) and `codec.go` (`//go:build !cgo` dummy stand-ins) — a real dual-build split, but only `internal/audio` has stubs; the `hraban/opus` codec dependency does not.
- `internal/client` — `CommsClient`: owns encoder/decoder, playback/capture streams, and RX/TX network ports; central orchestration point.
- `internal/network` — RTP receiver/transmitter over UDP multicast.
- `internal/rtp` — RTP packet parsing.
- `internal/config` — channel/port mapping logic (`TalkGroupPort`/`TalkGroupChannel`), validation, and defaults. Channel↔port math: `port = 38801 + (channel-1)*2`, channels 1–32 valid, only 8 shown in the TUI (`TUIChannelCount`).
- `internal/tui` — bubbletea model for the channel picker + hold-to-talk (falls back to toggle mode if the terminal lacks Kitty keyboard protocol support for key-release detection).
- `pkg/common` — shared logger setup (zerolog).

## Gotchas

- The app writes logs to `net-ptt.log` in the CWD at runtime (not stdout) because the TUI owns the terminal — don't expect log output on the console when running the binary.
- Only mono audio (`Channels == 1`) is accepted by `config.Validate`; sample rate must be exactly 48000/24000/16000.
- `bin/` is gitignored; a stray `bin/comms-client-macos` binary and `comms-client.log` file exist in the working tree already — don't assume they're build outputs to preserve.
</content>
