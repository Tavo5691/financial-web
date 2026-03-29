// Package uala_master parses Mastercard Ualá credit card statements.
package uala_master

import (
	"financial-web/internal/parser"
)

func init() {
	parser.Register(&ualaMasterParser{})
}

type ualaMasterParser struct{}

func (p *ualaMasterParser) ID() string { return "uala_master" }

func (p *ualaMasterParser) Info() parser.ParserInfo {
	return parser.ParserInfo{
		ID:          "uala_master",
		Bank:        "Ualá",
		CardNetwork: "Mastercard",
		Description: "Resumen Mastercard Ualá — formato digital",
	}
}

func (p *ualaMasterParser) Parse(text string) (parser.StatementMeta, []parser.ParsedTransaction, error) {
	meta := parser.StatementMeta{
		CardType: "MASTERCARD",
		CardBank: "UALA",
	}
	return meta, nil, nil
}
