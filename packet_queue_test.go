package sio

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tomruk/socket.io-go/engine.io/parser"
)

func TestPacketQueue(t *testing.T) {
	pq := newPacketQueue()

	pq.add(mustCreateEIOPacket(parser.PacketTypeMessage, false, nil))
	pq.add(mustCreateEIOPacket(parser.PacketTypePing, false, nil))

	packets := pq.get()
	assert.Equal(t, 2, len(packets))
	assert.Equal(t, 0, len(pq.packets))

	pq.add(mustCreateEIOPacket(parser.PacketTypeMessage, false, nil))
	pq.add(mustCreateEIOPacket(parser.PacketTypePing, false, nil))

	pq.reset()
	assert.Equal(t, 0, len(pq.packets))
}

func mustCreateEIOPacket(typ parser.PacketType, isBinary bool, data []byte) *parser.Packet {
	packet, err := parser.NewPacket(typ, isBinary, data)
	if err != nil {
		panic(err)
	}
	return packet
}
