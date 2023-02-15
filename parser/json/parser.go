package jsonparser

import (
	"github.com/tomruk/socket.io-go/parser"
	"github.com/tomruk/socket.io-go/parser/json/serializer"
)

// maxAttachments is the maximum number of the binary attachments to parse/send.
// If maxAttachments is 0, there will be no limit set for binary attachments.
func NewCreator(maxAttachments int, json serializer.JSONSerializer) parser.Creator {
	if json == nil {
		panic("sio: jsonparser.NewCreator: `json` must be set")
	}
	return func() parser.Parser {
		return &Parser{
			maxAttachments: maxAttachments,
			json:           json,
		}
	}
}

type Parser struct {
	r              *reconstructor
	maxAttachments int
	json           serializer.JSONSerializer
}

func (p *Parser) Reset() {
	p.r = nil
}
