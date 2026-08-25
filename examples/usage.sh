#!/bin/bash
# Usage examples for net-ptt

echo "=== Comms Client Usage Examples ==="
echo ""

echo "1. List available audio devices:"
echo "   ./net-ptt --list-devices"
echo ""

echo "2. Basic usage - launch the channel picker with default devices:"
echo "   ./net-ptt"
echo ""

echo "3. Launch with specific audio devices:"
echo "   ./net-ptt \\"
echo "     --output-device \"alsa_output.pci-0000_00_1f.3.analog-stereo\" \\"
echo "     --input-device \"alsa_input.pci-0000_00_1f.3.analog-stereo\""
echo ""

echo "4. Use a specific network interface:"
echo "   ./net-ptt --interface eth0"
echo ""

echo "5. Debug mode with statistics every 30 seconds:"
echo "   ./net-ptt \\"
echo "     --log-level DEBUG \\"
echo "     --stats-interval 30"
echo ""

echo "6. Low latency mode (smaller jitter buffer):"
echo "   ./net-ptt \\"
echo "     --jitter-depth 12 \\"
echo "     --min-latency 60"
echo ""

echo "7. High quality mode (higher bitrate):"
echo "   ./net-ptt \\"
echo "     --bitrate 64000 \\"
echo "     --complexity 10"
echo ""

echo "8. Loopback testing (receive own packets):"
echo "   ./net-ptt --loopback"
echo ""

echo "=== PTT Control (Terminal UI) ==="
echo "Use up/down (or j/k) to pick a channel."
echo "Hold SPACE to transmit, release to stop."
echo "If your terminal doesn't report key releases, SPACE toggles talk on/off."
echo "Press q or CTRL+C to quit."
echo ""
