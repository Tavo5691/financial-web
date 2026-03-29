package parser

import "time"

// ParsedTransaction is the raw output from a parser before DB insertion.
// Parsers must NOT touch the DB — they only return data.
type ParsedTransaction struct {
	Date                      time.Time
	DescriptionOriginal       string
	Amount                    float64
	Currency                  string // "ARS" or "USD"
	IsDebit                   bool
	IsInstallment             bool
	InstallmentCurrent        int
	InstallmentTotal          int
	InstallmentOriginalAmount float64
	// RawLine is the original text line — shown in validation output.
	RawLine string
}

// StatementMeta is extracted from the PDF header/footer by the parser.
type StatementMeta struct {
	CardType         string  // "VISA" or "MASTERCARD"
	CardBank         string  // "GALICIA", "UALA", "BBVA", etc.
	StatementPeriod  string  // "YYYY-MM"
	ExchangeRateUSD  float64 // 0 if statement has no USD
	DeclaredTotalARS float64 // total from statement footer; 0 if not found
}

// ParserInfo is displayed in the /parsers web page.
type ParserInfo struct {
	ID          string
	Bank        string
	CardNetwork string
	Description string
}

// FileParser is an optional extension of Parser for PDFs that cannot be
// extracted with the standard text extractor (e.g. BBVA XObject-based PDFs).
// When a parser implements this interface, the service and CLI call ParseFile
// directly instead of extracting text first and calling Parse.
type FileParser interface {
	Parser
	ParseFile(path string) (StatementMeta, []ParsedTransaction, error)
}

// Parser is the interface every bank-specific parser must implement.
type Parser interface {
	// ID returns the stable unique identifier, e.g. "galicia_visa".
	// Used as a FK in the statements table and as the CLI argument.
	ID() string

	// Info returns human-readable metadata for the UI.
	Info() ParserInfo

	// Parse extracts transactions from the full PDF text (all pages joined).
	// Non-fatal issues should be returned as warnings inside ParsedTransaction.RawLine
	// or surfaced via the error return for fatal failures.
	Parse(text string) (StatementMeta, []ParsedTransaction, error)
}
