//go:build cgo

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/gen2brain/malgo"
)

func listMalgoDevices() {
	fmt.Println("============================================================")
	fmt.Println("Available Audio Devices (malgo/PortAudio)")
	fmt.Println("============================================================")

	// Initialize malgo context
	malgoCtx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init malgo: %v\n", err)
		return
	}
	defer malgoCtx.Uninit()

	// Get playback devices
	fmt.Println("\n[PLAYBACK DEVICES]")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("%-6s %-40s %s\n", "Index", "Name", "ID")
	fmt.Println(strings.Repeat("-", 60))

	playbackDevs, err := malgoCtx.Devices(malgo.Playback)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get playback devices: %v\n", err)
		return
	}

	for i, dev := range playbackDevs {
		fmt.Printf("%-6d %-40s %s\n", i, dev.Name(), dev.ID)
	}

	// Get capture devices
	fmt.Println("\n[CAPTURE DEVICES]")
	fmt.Println(strings.Repeat("-", 60))
	fmt.Printf("%-6s %-40s %s\n", "Index", "Name", "ID")
	fmt.Println(strings.Repeat("-", 60))

	captureDevs, err := malgoCtx.Devices(malgo.Capture)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get capture devices: %v\n", err)
		return
	}

	for i, dev := range captureDevs {
		fmt.Printf("%-6d %-40s %s\n", i, dev.Name(), dev.ID)
	}

	fmt.Println("\n============================================================")
	fmt.Println("Device Selection Guide:")
	fmt.Println("============================================================")
	fmt.Println("• Use index number from above list")
	fmt.Println("• Use 'default' for system default device")
	fmt.Println("• Example: --output-device 0 or --device default")
	fmt.Printf("• Total devices found: %d\n", len(playbackDevs)+len(captureDevs))
	fmt.Println("")
}
