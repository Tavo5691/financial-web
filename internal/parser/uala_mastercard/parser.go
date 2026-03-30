// Package uala_mastercard parses Mastercard Ualá credit card statements.
//
// The PDF uses standard text encoding (no XObject). GetPlainText produces
// transactions in order: date → pre-desc lines → post-desc lines → coupon → amount.
// Column positions (Pesos x≈361 vs Dólares x≈452) are detected via positioned
// text extraction and used to classify currency for each amount.
package uala_mastercard

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"financial-web/internal/parser"
	"financial-web/internal/pdf"
)

func init() {
	parser.Register(&ualaMastercardParser{})
}

type ualaMastercardParser struct{}

func (p *ualaMastercardParser) ID() string { return "uala_mastercard" }

func (p *ualaMastercardParser) Info() parser.ParserInfo {
	return parser.ParserInfo{
		ID:          "uala_mastercard",
		Bank:        "Ualá",
		CardNetwork: "Mastercard",
		Description: "Resumen Mastercard Ualá — formato digital",
	}
}

// Parse satisfies parser.Parser but is not called directly; ParseFile is used instead.
func (p *ualaMastercardParser) Parse(text string) (parser.StatementMeta, []parser.ParsedTransaction, error) {
	return parser.StatementMeta{CardType: "MASTERCARD", CardBank: "UALA"}, nil, nil
}

func (p *ualaMastercardParser) ParseFile(path string) (parser.StatementMeta, []parser.ParsedTransaction, error) {
	text, err := pdf.ExtractText(path)
	if err != nil {
		return parser.StatementMeta{}, nil, fmt.Errorf("extract text: %w", err)
	}

	usdAmounts := extractUSDAmounts(path) // best-effort; nil on error

	return parseText(text, usdAmounts)
}

// ── USD column detection ──────────────────────────────────────────────────────

// extractUSDAmounts scans positioned text for amount strings that appear in the
// Dólares column (x ≈ 452, empirical range [445, 530]).  Returns the set of
// raw amount strings (as they appear in GetPlainText) that are denominated in USD.
func extractUSDAmounts(path string) map[string]bool {
	items, err := pdf.ExtractPositioned(path)
	if err != nil {
		return nil
	}
	rows := pdf.GroupByRow(items, 3.0)

	result := map[string]bool{}
	for _, row := range rows {
		// Only consider rows that have a digit in the Date column (x ≈ 67).
		// This excludes header rows ("Fecha", "Total…") and section titles.
		if !pdf.HasDigitInColumn(row, 60, 115) {
			continue
		}

		// Assemble chars in the Dólares column (x ≈ 452).
		amtStr := pdf.CollectColumnText(row, 445, 530)
		if amtStr == "" {
			continue
		}
		// Accept only strings that look like a standalone amount (not "USD26,98").
		check := strings.TrimPrefix(amtStr, "-")
		if reAmount.MatchString(check) {
			result[amtStr] = true
		}
	}
	return result
}

// ── regexes ───────────────────────────────────────────────────────────────────

var (
	// Transaction date in body: "01 ENE 26"  (2-digit year, uppercase month)
	reDate = regexp.MustCompile(`^(\d{2})\s+([A-ZÁÉÍÓÚ]{3})\s+(\d{2})$`)
	// Header closing-date: "30 Ene 2026"  (4-digit year, mixed-case month)
	reHeaderDate = regexp.MustCompile(`^(\d{1,2})\s+([A-Za-záéíóú]{3,})\s+(\d{4})$`)
	// Cupón: letter + 5 digits, or literal "0"
	reCupon = regexp.MustCompile(`^[A-Za-z]\d{5}$`)
	// Argentine numeric amount: optional minus, dot-thousands, comma-decimal
	reAmount = regexp.MustCompile(`^-?[\d.]+,\d{2}$`)
)

var spanishMonths = map[string]int{
	"ene": 1, "feb": 2, "mar": 3, "abr": 4, "may": 5, "jun": 6,
	"jul": 7, "ago": 8, "sep": 9, "set": 9, "oct": 10, "nov": 11, "dic": 12,
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseDate(s string) (time.Time, bool) {
	m := reDate.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return time.Time{}, false
	}
	day, _ := strconv.Atoi(m[1])
	month, ok := spanishMonths[strings.ToLower(m[2])]
	if !ok {
		return time.Time{}, false
	}
	year, _ := strconv.Atoi(m[3])
	if year < 100 {
		year += 2000
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), true
}

func parseHeaderDate(s string) (time.Time, bool) {
	m := reHeaderDate.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return time.Time{}, false
	}
	day, _ := strconv.Atoi(m[1])
	month, ok := spanishMonths[strings.ToLower(m[2][:3])]
	if !ok {
		return time.Time{}, false
	}
	year, _ := strconv.Atoi(m[3])
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), true
}

func parseAmount(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	if neg {
		v = -v
	}
	return v, true
}

func roundCents(v float64) float64 { return math.Round(v*100) / 100 }

// shouldSkipTax returns true for the RG 5463 perception lines that net to zero
// because the user pays their USD balance in USD (bank applies + then reverses it).
func shouldSkipTax(desc string) bool {
	return strings.Contains(strings.ToUpper(desc), "RG 5463")
}

// ── section type ──────────────────────────────────────────────────────────────

type section int

const (
	sNone section = iota
	sConsumos
	sPagos
	sTaxes
)

// ── main plain-text parser ────────────────────────────────────────────────────

func parseText(text string, usdAmounts map[string]bool) (parser.StatementMeta, []parser.ParsedTransaction, error) {
	meta := parser.StatementMeta{CardType: "MASTERCARD", CardBank: "UALA"}
	var txns []parser.ParsedTransaction

	// currency returns "USD" if the raw amount string is in the USD set,
	// otherwise "ARS".
	currency := func(rawAmt string) string {
		if usdAmounts[rawAmt] {
			return "USD"
		}
		return "ARS"
	}

	// Consumos accumulation state.
	var (
		pendingDate time.Time
		descParts   []string
		hasCupon    bool
	)
	resetConsumos := func() {
		pendingDate = time.Time{}
		descParts = nil
		hasCupon = false
	}
	emitConsumos := func(rawAmt string, amt float64) {
		desc := strings.TrimSpace(strings.Join(descParts, " "))
		isDebit := amt >= 0
		if amt < 0 {
			amt = -amt
		}
		txns = append(txns, parser.ParsedTransaction{
			Date:                pendingDate,
			DescriptionOriginal: desc,
			Amount:              roundCents(amt),
			Currency:            currency(rawAmt),
			IsDebit:             isDebit,
			RawLine:             desc,
		})
		resetConsumos()
	}

	// Taxes accumulation state.
	var (
		taxDate     time.Time
		taxDescParts []string
	)
	resetTax := func() {
		taxDate = time.Time{}
		taxDescParts = nil
	}
	emitTax := func(amt float64) {
		desc := strings.TrimSpace(strings.Join(taxDescParts, " "))
		if shouldSkipTax(desc) {
			resetTax()
			return
		}
		isDebit := amt >= 0
		if amt < 0 {
			amt = -amt
		}
		txns = append(txns, parser.ParsedTransaction{
			Date:                taxDate,
			DescriptionOriginal: desc,
			Amount:              roundCents(amt),
			Currency:            "ARS",
			IsDebit:             isDebit,
			RawLine:             desc,
		})
		resetTax()
	}

	sec := sNone
	closingDateNext := false

	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		// ── StatementPeriod detection (independent of section) ───────────────
		if meta.StatementPeriod == "" {
			if line == "Fecha de cierre" {
				closingDateNext = true
				continue
			}
			if closingDateNext {
				if d, ok := parseHeaderDate(line); ok {
					meta.StatementPeriod = d.Format("2006-01")
				}
				closingDateNext = false
				// fall through: the line may also be processed by section logic
				// (unlikely, but safe to continue)
				continue
			}
		}

		// ── Section transitions ───────────────────────────────────────────────
		switch line {
		case "Consumos":
			resetConsumos()
			sec = sConsumos
			continue
		case "Pagos":
			resetConsumos()
			sec = sPagos
			continue
		case "Desconocimiento de compra":
			sec = sNone
			goto done
		}
		if strings.HasPrefix(line, "Impuestos") {
			resetTax()
			sec = sTaxes
			continue
		}

		switch sec {
		case sConsumos:
			// Skip table headers and subtotal lines.
			switch line {
			case "Fecha", "Descripción", "N° Cupón", "Pesos", "Dólares",
				"Total", "Pago en pesos", "Pago en dólares":
				continue
			}
			if strings.HasPrefix(line, "$ ") || strings.HasPrefix(line, "USD ") ||
				strings.HasPrefix(line, "-$ ") || strings.HasPrefix(line, "-USD ") {
				continue
			}

			// Date line → start a new transaction.
			if d, ok := parseDate(line); ok {
				resetConsumos()
				pendingDate = d
				continue
			}

			if pendingDate.IsZero() {
				continue // haven't seen a date yet, ignore
			}

			// Amount line → emit transaction.
			if reAmount.MatchString(line) {
				amt, ok := parseAmount(line)
				if ok {
					emitConsumos(line, amt)
				}
				continue
			}

			// Cupón line.
			if reCupon.MatchString(line) || line == "0" {
				hasCupon = true
				_ = hasCupon
				continue
			}

			// Everything else is description text.
			descParts = append(descParts, line)

		case sTaxes:
			// Skip table headers and total lines.
			switch line {
			case "Fecha", "Descripción", "Pago en pesos", "Pago en dólares", "Total":
				continue
			}
			if strings.HasPrefix(line, "$ ") || strings.HasPrefix(line, "USD ") ||
				strings.HasPrefix(line, "-$ ") || strings.HasPrefix(line, "-USD ") {
				continue
			}

			// Date line.
			if d, ok := parseDate(line); ok {
				resetTax()
				taxDate = d
				continue
			}

			if taxDate.IsZero() {
				continue
			}

			// Amount line → emit tax transaction.
			if reAmount.MatchString(line) {
				amt, ok := parseAmount(line)
				if ok {
					emitTax(amt)
				}
				continue
			}

			// Description continuation.
			taxDescParts = append(taxDescParts, line)
		}
	}

done:
	return meta, txns, nil
}
