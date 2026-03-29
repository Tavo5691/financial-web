// Package galicia_mastercard_mas parses Mastercard Galicia Más credit card statements.
package galicia_mastercard_mas

import (
	"financial-web/internal/parser"
)

func init() {
	parser.Register(&galiciaMastercardMasParser{})
}

type galiciaMastercardMasParser struct{}

func (p *galiciaMastercardMasParser) ID() string { return "galicia_mastercard_mas" }

func (p *galiciaMastercardMasParser) Info() parser.ParserInfo {
	return parser.ParserInfo{
		ID:          "galicia_mastercard_mas",
		Bank:        "Galicia",
		CardNetwork: "Mastercard Más",
		Description: "Resumen Mastercard Galicia Más — formato digital",
	}
}

func (p *galiciaMastercardMasParser) Parse(text string) (parser.StatementMeta, []parser.ParsedTransaction, error) {
	meta := parser.StatementMeta{
		CardType: "MASTERCARD",
		CardBank: "GALICIA",
	}
	return meta, nil, nil
}
