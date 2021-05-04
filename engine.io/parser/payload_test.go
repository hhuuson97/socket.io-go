package parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeDecodePayloads(t *testing.T) {
	test := []*Packet{
		mustCreatePacket(t, PacketTypeOpen, false, nil),
		mustCreatePacket(t, PacketTypeClose, false, nil),
		mustCreatePacket(t, PacketTypePing, false, []byte("testing123")),
		mustCreatePacket(t, PacketTypePong, false, []byte("testing123")),
		mustCreatePacket(t, PacketTypeMessage, false, []byte("testing123")),
		mustCreatePacket(t, PacketTypeMessage, true, []byte{0x0, 0x1, 0x2, 0x3}),
		mustCreatePacket(t, PacketTypeUpgrade, false, nil),
		mustCreatePacket(t, PacketTypeNoop, false, nil),
	}

	buf := bytes.NewBuffer(nil)
	l := EncodedPayloadsLen(test...)
	err := EncodePayloads(buf, test...)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, l, buf.Len())

	if !assert.Greater(t, buf.Len(), 0) {
		return
	}

	packets, err := DecodePayloads(buf)
	if err != nil {
		t.Fatal(err)
	}

	for i, p1 := range packets {
		p2 := test[i]

		assert.Equal(t, p2.Type, p1.Type, "packet type doesn't match")
		assert.Equal(t, p2.IsBinary, p1.IsBinary, "isBinary doesn't match")

		if !bytes.Equal(p1.Data, p2.Data) {
			t.Fatal("packet data doesn't match")
		}
	}
}

func TestSplitByte(t *testing.T) {
	const delim byte = '|'

	tests := [][]byte{
		[]byte(""),
		[]byte("|"),
		[]byte("|||"),
		[]byte("123|"),
		[]byte("|123"),
		[]byte("123"),
		[]byte("123|456"),
		[]byte("123|456|789"),
	}

	expected := [][][]byte{
		{
			[]byte(""),
		},
		{
			[]byte(""),
			[]byte(""),
		},
		{
			[]byte(""),
			[]byte(""),
			[]byte(""),
			[]byte(""),
		},
		{
			[]byte("123"),
			[]byte(""),
		},
		{
			[]byte(""),
			[]byte("123"),
		},
		{
			[]byte("123"),
		},
		{
			[]byte("123"),
			[]byte("456"),
		},
		{
			[]byte("123"),
			[]byte("456"),
			[]byte("789"),
		},
	}

	for i, test := range tests {
		splitted := splitByte(test, delim)

		/*
			for _i, s := range splitted {
				fmt.Printf("splitted[%d]: %s\n", _i, s)
			}
			fmt.Print("\n")
		*/

		if !assert.Equal(t, len(expected[i]), len(splitted), "expected and splitted should be equal") {
			return
		}

		for j, e := range expected[i] {
			if !bytes.Equal(splitted[j], e) {
				t.Fatal("test and expected should match")
			}
		}
	}

}

func TestDecodeSinglePayload(t *testing.T) {
	test := mustCreatePacket(t, PacketTypeMessage, true, []byte{0x0, 0x1, 0x2, 0x3})

	buf := bytes.NewBuffer(nil)
	err := EncodePayloads(buf, test)
	if err != nil {
		t.Fatal(err)
	}

	if !assert.Greater(t, buf.Len(), 0) {
		return
	}

	packets, err := DecodePayloads(buf)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 1, len(packets))

	p1 := test
	p2 := packets[0]

	assert.Equal(t, p2.Type, p1.Type, "packet type doesn't match")
	assert.Equal(t, p2.IsBinary, p1.IsBinary, "isBinary doesn't match")

	if !bytes.Equal(p1.Data, p2.Data) {
		t.Fatal("packet data doesn't match")
	}
}

func TestDecodeInvalidPayload(t *testing.T) {
	test := mustCreatePacket(t, PacketTypeMessage, true, []byte{0x0, 0x1, 0x2, 0x3})

	buf := bytes.NewBuffer(nil)
	err := EncodePayloads(buf, test)
	if err != nil {
		t.Fatal(err)
	}

	if !assert.Greater(t, buf.Len(), 0) {
		return
	}

	buf.Bytes()[0] = 2 // Lower than 48. To provoke errInvalidPacketType.

	_, err = DecodePayloads(buf)
	if err != errInvalidPacketType {
		t.Fatal("errInvalidPacketType expected")
	}
}
