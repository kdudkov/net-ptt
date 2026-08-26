package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/hraban/opus"
	"github.com/kdudkov/net-ptt/internal/config"
	"github.com/kdudkov/net-ptt/internal/rtp"
)

func main() {
	var (
		flagFile          = flag.String("file", "", "WAV file to transmit [REQUIRED]")
		flagMulticastAddr = flag.String("multicast-addr", "239.192.41.1", "Multicast address")
		flagChannel       = flag.Int("channel", 1, "Talk group channel (1-32)")
		flagRepeat        = flag.Int("repeat", 1, "Repeat count")
		flagGain          = flag.Float64("gain", 1.0, "Audio gain multiplier")
	)

	flag.Parse()

	if *flagFile == "" {
		fmt.Fprintln(os.Stderr, "Error: --file is required")
		flag.Usage()
		os.Exit(1)
	}

	port, err := config.TalkGroupPort(*flagChannel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid channel: %v\n", err)
		os.Exit(1)
	}

	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(*flagMulticastAddr, port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: resolve addr: %v\n", err)
		os.Exit(1)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: dial UDP: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	data, sampleRate, channels, err := readWAV(*flagFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: read WAV: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("WAV: %d Hz, %d ch, %d samples\n", sampleRate, channels, len(data))

	// Convert to mono if stereo
	var pcm []int16
	if channels == 2 {
		pcm = stereoToMono(data)
	} else {
		pcm = data
	}

	// Apply gain
	if *flagGain != 1.0 {
		for i := range pcm {
			s := float64(pcm[i]) * *flagGain
			if s > 32767 {
				s = 32767
			} else if s < -32768 {
				s = -32768
			}
			pcm[i] = int16(s)
		}
	}

	enc, err := opus.NewEncoder(sampleRate, 1, opus.AppVoIP)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: create encoder: %v\n", err)
		os.Exit(1)
	}

	frameSize := sampleRate / 50 // 20ms frames

	for r := 0; r < *flagRepeat; r++ {
		seq := uint16(0)
		ts := uint32(0)
		ssrc := uint32(0x12345678)

		for i := 0; i+frameSize <= len(pcm); i += frameSize {
			frame := pcm[i : i+frameSize]
			out := make([]byte, 400)
			n, err := enc.Encode(frame, out)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Encode error: %v\n", err)
				continue
			}

			marker := (r == 0 && i == 0) || i == 0
			header := rtp.BuildRTPHeader(seq, ts, ssrc, rtp.OpusPayloadType, marker)
			packet := append(header, out[:n]...)

			_, err = conn.Write(packet)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Write error: %v\n", err)
				continue
			}

			seq++
			ts += uint32(frameSize)

			// Sleep to maintain real-time rate
			time.Sleep(20 * time.Millisecond)
		}

		fmt.Printf("Transmitted %d frames (repeat %d/%d)\n", seq, r+1, *flagRepeat)
	}
}

func readWAV(path string) ([]int16, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()

	// RIFF header
	var riff [4]byte
	if _, err := f.Read(riff[:]); err != nil {
		return nil, 0, 0, err
	}
	if string(riff[:]) != "RIFF" {
		return nil, 0, 0, fmt.Errorf("not a RIFF file")
	}

	var fileSize uint32
	binary.Read(f, binary.LittleEndian, &fileSize)

	var wave [4]byte
	if _, err := f.Read(wave[:]); err != nil {
		return nil, 0, 0, err
	}
	if string(wave[:]) != "WAVE" {
		return nil, 0, 0, fmt.Errorf("not a WAVE file")
	}

	var sampleRate, channels int
	var dataOffset int64
	var dataSize uint32

	for {
		var chunkID [4]byte
		_, err := f.Read(chunkID[:])
		if err != nil {
			break
		}

		var chunkSize uint32
		binary.Read(f, binary.LittleEndian, &chunkSize)

		switch string(chunkID[:]) {
		case "fmt ":
			var audioFormat uint16
			binary.Read(f, binary.LittleEndian, &audioFormat)
			if audioFormat != 1 {
				return nil, 0, 0, fmt.Errorf("unsupported audio format: %d", audioFormat)
			}

			var ch uint16
			binary.Read(f, binary.LittleEndian, &ch)
			channels = int(ch)

			var sr uint32
			binary.Read(f, binary.LittleEndian, &sr)
			sampleRate = int(sr)

			// skip byteRate (4), blockAlign (2)
			f.Seek(6, 1)

			var bitsPerSample uint16
			binary.Read(f, binary.LittleEndian, &bitsPerSample)
			if bitsPerSample != 16 {
				return nil, 0, 0, fmt.Errorf("unsupported bits per sample: %d", bitsPerSample)
			}

			// Skip any extra fmt bytes
			if chunkSize > 16 {
				f.Seek(int64(chunkSize-16), 1)
			}

		case "data":
			dataOffset, _ = f.Seek(0, 1)
			dataSize = chunkSize
			goto foundData

		default:
			f.Seek(int64(chunkSize), 1)
		}
	}

foundData:
	if dataOffset == 0 {
		return nil, 0, 0, fmt.Errorf("no data chunk found")
	}

	f.Seek(dataOffset, 0)
	buf := make([]byte, dataSize)
	_, err = f.Read(buf)
	if err != nil {
		return nil, 0, 0, err
	}

	samples := make([]int16, len(buf)/2)
	for i := range samples {
		samples[i] = int16(buf[i*2]) | int16(buf[i*2+1])<<8
	}

	return samples, sampleRate, channels, nil
}

func stereoToMono(stereo []int16) []int16 {
	mono := make([]int16, len(stereo)/2)
	for i := range mono {
		mono[i] = (stereo[i*2] + stereo[i*2+1]) / 2
	}
	return mono
}
