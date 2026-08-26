package rtp

import (
	"encoding/binary"
	"errors"
)

const (
	OpusPayloadType = 111
)

type Packet struct {
	Header  Header
	Payload []byte
}

type Header struct {
	Version     uint8
	Padding     bool
	Extension   bool
	CSRCCount   uint8
	Marker      bool
	PayloadType uint8
	Sequence    uint16
	Timestamp   uint32
	SSRC        uint32
}

func ParsePacket(data []byte) (*Packet, error) {
	if len(data) < 12 {
		return nil, errors.New("RTP packet too short")
	}

	packet := &Packet{}

	packet.Header.CSRCCount = data[0] & 0x0F
	packet.Header.Marker = (data[1]>>7)&0x01 == 1
	packet.Header.Sequence = binary.BigEndian.Uint16(data[2:4])
	packet.Header.Timestamp = binary.BigEndian.Uint32(data[4:8])
	packet.Header.SSRC = binary.BigEndian.Uint32(data[8:12])

	payloadStart := 12 + int(packet.Header.CSRCCount)*4
	if payloadStart > len(data) {
		return nil, errors.New("RTP payload out of bounds")
	}

	packet.Payload = data[payloadStart:]

	return packet, nil
}

func BuildRTPHeader(sequence uint16, timestamp uint32, ssrc uint32, payloadType uint8, marker bool) []byte {
	buf := make([]byte, 12)

	buf[0] = (2 << 6)
	if marker {
		buf[1] = 0x80 | payloadType
	} else {
		buf[1] = payloadType
	}

	binary.BigEndian.PutUint16(buf[2:4], sequence)
	binary.BigEndian.PutUint32(buf[4:8], timestamp)
	binary.BigEndian.PutUint32(buf[8:12], ssrc)

	return buf
}
