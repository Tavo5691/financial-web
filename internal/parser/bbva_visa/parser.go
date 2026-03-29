// Package bbva_visa parses Visa BBVA credit card statements.
// BBVA PDFs store content in XObject form streams which ledongthuc/pdf cannot
// extract; this parser uses pdfcpu via pdf.ExtractTokens instead.
package bbva_visa

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
	parser.Register(&bbvaVisaParser{})
}

type bbvaVisaParser struct{}

func (p *bbvaVisaParser) ID() string { return "bbva_visa" }

func (p *bbvaVisaParser) Info() parser.ParserInfo {
	return parser.ParserInfo{
		ID:          "bbva_visa",
		Bank:        "BBVA",
		CardNetwork: "Visa",
		Description: "Resumen Visa BBVA — formato digital",
	}
}

// Parse is never called for BBVA — ParseFile is used instead.
func (p *bbvaVisaParser) Parse(text string) (parser.StatementMeta, []parser.ParsedTransaction, error) {
	return parser.StatementMeta{CardType: "VISA", CardBank: "BBVA"}, nil, nil
}

func (p *bbvaVisaParser) ParseFile(path string) (parser.StatementMeta, []parser.ParsedTransaction, error) {
	tokens, err := pdf.ExtractTokens(path)
	if err != nil {
		return parser.StatementMeta{}, nil, fmt.Errorf("extract tokens: %w", err)
	}
	return parseTokens(tokens)
}

var (
	reDate        = regexp.MustCompile(`^\d{2}-[A-Za-z]{3}-\d{2}$`)
	reCoupon      = regexp.MustCompile(`^\d{6}$`)
	reInstallment = regexp.MustCompile(`\s+C\.(\d+)/(\d+)\s*$`)
	// USD transactions: description ends with "<id>USD   <amount>" column padding.
	reUSD = regexp.MustCompile(`^(.+?)\s+\S+USD\s+([\d,]+)\s*$`)
)

var spanishMonths = map[string]int{
	"ene": 1, "feb": 2, "mar": 3, "abr": 4, "may": 5, "jun": 6,
	"jul": 7, "ago": 8, "sep": 9, "set": 9, "oct": 10, "nov": 11, "dic": 12,
}

func parseDate(s string) (time.Time, error) {
	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("invalid date %q", s)
	}
	day, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, err
	}
	month, ok := spanishMonths[strings.ToLower(parts[1])]
	if !ok {
		return time.Time{}, fmt.Errorf("unknown month %q", parts[1])
	}
	year, err := strconv.Atoi(parts[2])
	if err != nil {
		return time.Time{}, err
	}
	if year < 100 {
		year += 2000
	}
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC), nil
}

// parseAmount converts Argentine numeric format ("1.234,56") to float64.
func parseAmount(s string) (float64, error) {
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	return strconv.ParseFloat(s, 64)
}

func roundCents(v float64) float64 {
	return math.Round(v*100) / 100
}

func parseTokens(tokens []string) (parser.StatementMeta, []parser.ParsedTransaction, error) {
	meta := parser.StatementMeta{CardType: "VISA", CardBank: "BBVA"}

	// Extract statement period from "CIERRE ACTUAL" + date token on page 1.
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i] == "CIERRE ACTUAL" && reDate.MatchString(tokens[i+1]) {
			if d, err := parseDate(tokens[i+1]); err == nil {
				meta.StatementPeriod = d.Format("2006-01")
			}
			break
		}
	}

	var txns []parser.ParsedTransaction
	var iibbDate time.Time
	var iibbTotal float64
	var ivaDate time.Time
	var ivaTotal float64

	// inTransactions: set after "Consumos <name>" section header on page 1.
	// inTaxes: set after "Impuestos, cargos e intereses" header.
	inTransactions := false
	inTaxes := false

	i := 0
	for i < len(tokens) {
		tok := tokens[i]

		// --- Section boundary detection ---
		if !inTransactions && strings.HasPrefix(tok, "Consumos ") {
			inTransactions = true
			i++
			continue
		}
		if inTransactions && !inTaxes && tok == "Impuestos, cargos e intereses" {
			inTaxes = true
			i++
			continue
		}
		if inTaxes && tok == "SALDO ACTUAL" {
			break
		}

		// Skip until we're in an active section, or skip non-date noise tokens.
		if !inTransactions || !reDate.MatchString(tok) {
			i++
			continue
		}

		// --- Parse a row starting with a date token ---
		date, err := parseDate(tok)
		if err != nil {
			i++
			continue
		}
		i++
		if i >= len(tokens) {
			break
		}
		desc := tokens[i]
		i++

		// A 6-digit coupon follows description for regular transactions.
		// Tax rows have no coupon.
		if i < len(tokens) && reCoupon.MatchString(tokens[i]) {
			i++ // consume coupon, we don't store it
		}

		if i >= len(tokens) {
			break
		}
		amountStr := tokens[i]
		i++

		amt, err := parseAmount(amountStr)
		if err != nil {
			continue
		}

		// Skip payments and RG 5617 credit/debit rows.
		if strings.Contains(desc, "SU PAGO") {
			continue
		}
		if strings.Contains(desc, "RG 5617") {
			continue
		}

		if inTaxes {
			// Aggregate IIBB and IVA lines; skip the rest.
			switch {
			case strings.HasPrefix(desc, "IIBB"):
				if iibbDate.IsZero() {
					iibbDate = date
				}
				iibbTotal += amt
			case strings.HasPrefix(desc, "IVA RG 4240"):
				if ivaDate.IsZero() {
					ivaDate = date
				}
				ivaTotal += amt
			default:
				txns = append(txns, parser.ParsedTransaction{
					Date:                date,
					DescriptionOriginal: strings.TrimSpace(desc),
					Amount:              amt,
					Currency:            "ARS",
					IsDebit:             true,
					RawLine:             desc,
				})
			}
			continue
		}

		txns = append(txns, buildTxn(date, desc, amt))
	}

	// Emit single aggregated IIBB and IVA transactions.
	if iibbTotal > 0 {
		txns = append(txns, parser.ParsedTransaction{
			Date:                iibbDate,
			DescriptionOriginal: "IIBB PERCEP-CABA (consumos en USD)",
			Amount:              roundCents(iibbTotal),
			Currency:            "ARS",
			IsDebit:             true,
			RawLine:             "IIBB PERCEP-CABA (consumos en USD)",
		})
	}
	if ivaTotal > 0 {
		txns = append(txns, parser.ParsedTransaction{
			Date:                ivaDate,
			DescriptionOriginal: "IVA RG 4240 (consumos en USD)",
			Amount:              roundCents(ivaTotal),
			Currency:            "ARS",
			IsDebit:             true,
			RawLine:             "IVA RG 4240 (consumos en USD)",
		})
	}

	return meta, txns, nil
}

func buildTxn(date time.Time, rawDesc string, amt float64) parser.ParsedTransaction {
	txn := parser.ParsedTransaction{
		Date:     date,
		Currency: "ARS",
		IsDebit:  amt >= 0,
		RawLine:  rawDesc,
	}
	if amt < 0 {
		txn.Amount = -amt
	} else {
		txn.Amount = amt
	}

	// USD transactions: description contains "<id>USD   <amount>" suffix.
	if m := reUSD.FindStringSubmatch(rawDesc); m != nil {
		usdAmt, err := parseAmount(m[2])
		if err == nil {
			txn.DescriptionOriginal = strings.TrimSpace(m[1])
			txn.Amount = usdAmt
			txn.Currency = "USD"
			txn.IsDebit = true
			return txn
		}
	}

	// Installments: description ends with "C.N/M".
	if loc := reInstallment.FindStringSubmatchIndex(rawDesc); loc != nil {
		groups := reInstallment.FindStringSubmatch(rawDesc)
		cur, _ := strconv.Atoi(groups[1])
		tot, _ := strconv.Atoi(groups[2])
		txn.IsInstallment = true
		txn.InstallmentCurrent = cur
		txn.InstallmentTotal = tot
		txn.DescriptionOriginal = strings.TrimSpace(rawDesc[:loc[0]])
	} else {
		txn.DescriptionOriginal = strings.TrimSpace(rawDesc)
	}

	return txn
}
