package parser

import (
	"encoding/json"
	"fmt"
)

var errInvalidPacketType = fmt.Errorf("parser: invalid packet type")

type PacketType byte

const (
	PacketTypeConnect PacketType = iota
	PacketTypeDisconnect
	PacketTypeEvent
	PacketTypeAck
	PacketTypeConnectError
	PacketTypeBinaryEvent
	PacketTypeBinaryAck

	packetTypeMin = PacketTypeConnect
	packetTypeMax = PacketTypeBinaryAck
)

func (p PacketType) ToChar() byte {
	b := byte(p)
	b += 48
	return b
}

func (p *PacketType) FromChar(b byte) error {
	if b < 48 || b > byte(48+packetTypeMax) {
		return errInvalidPacketType
	}

	b = b - 48
	*p = PacketType(b)
	return nil
}

type PacketHeader struct {
	Type        PacketType
	Namespace   string
	ID          *uint64
	Attachments int
}

func (p *PacketHeader) IsBinary() bool {
	return p.Type == PacketTypeBinaryEvent || p.Type == PacketTypeBinaryAck
}

func (p *PacketHeader) IsEvent() bool {
	return p.Type == PacketTypeEvent || p.Type == PacketTypeBinaryEvent
}

func (p *PacketHeader) IsAck() bool {
	return p.Type == PacketTypeAck || p.Type == PacketTypeBinaryAck
}

func (p PacketHeader) MarshalBinary() ([]byte, error) {
	return json.Marshal(&p)
}

func (p *PacketHeader) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, &p)
}
