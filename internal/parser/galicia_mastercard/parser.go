// Package galicia_mastercard parses Mastercard Galicia credit card statements.
package galicia_mastercard

import (
	"financial-web/internal/parser"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() {
	parser.Register(&galiciaMastercardParser{})
}

type galiciaMastercardParser struct{}

func (p *galiciaMastercardParser) ID() string { return "galicia_mastercard" }

func (p *galiciaMastercardParser) Info() parser.ParserInfo {
	return parser.ParserInfo{
		ID:          "galicia_mastercard",
		Bank:        "Galicia",
		CardNetwork: "Mastercard",
		Description: "Resumen Mastercard Galicia — formato digital",
	}
}

var (
	// reDateLine matches transaction date lines: DD-Mmm-YY (Spanish abbreviated month)
	reDateLine = regexp.MustCompile(`^(\d{2})-(Ene|Feb|Mar|Abr|May|Jun|Jul|Ago|Sep|Oct|Nov|Dic)-(\d{2})$`)
	// reCuotaInDesc extracts installment notation from end of description, e.g. "PSA   06/12"
	reCuotaInDesc = regexp.MustCompile(`^(.+?)\s+(\d{1,2})/(\d{2})\s*$`)
	// rePeriod extracts YYYYMM from the statement ID line (e.g. "20260131060271011H")
	rePeriod = regexp.MustCompile(`(\d{4})(\d{2})\d{2}\d+H`)
	// reTotalPagar finds the declared total to pay
	reTotalPagar = regexp.MustCompile(`(?m)TOTAL A PAGAR\s*\n\s*([\d.]+,\d+)`)
)

var spanishMonths = map[string]string{
	"Ene": "01", "Feb": "02", "Mar": "03", "Abr": "04",
	"May": "05", "Jun": "06", "Jul": "07", "Ago": "08",
	"Sep": "09", "Oct": "10", "Nov": "11", "Dic": "12",
}

// parseAmount converts Argentine number format ("1.234,56") to float64.
func parseAmount(s string) (float64, error) {
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	return strconv.ParseFloat(s, 64)
}

// parseDate converts "DD-Mmm-YY" to time.Time.
func parseDate(s string) (time.Time, error) {
	m := reDateLine.FindStringSubmatch(s)
	if m == nil {
		return time.Time{}, strconv.ErrSyntax
	}
	month, ok := spanishMonths[m[2]]
	if !ok {
		return time.Time{}, strconv.ErrSyntax
	}
	return time.Parse("02-01-06", m[1]+"-"+month+"-"+m[3])
}

func (p *galiciaMastercardParser) Parse(text string) (parser.StatementMeta, []parser.ParsedTransaction, error) {
	meta := parser.StatementMeta{
		CardType: "MASTERCARD",
		CardBank: "GALICIA",
	}

	// Extract period from statement ID line like "20260131060271011H" → "2026-01"
	if m := rePeriod.FindStringSubmatch(text); m != nil {
		meta.StatementPeriod = m[1] + "-" + m[2]
	}

	// Extract declared total from "TOTAL A PAGAR\n172.926,85"
	if m := reTotalPagar.FindStringSubmatch(text); m != nil {
		if v, err := parseAmount(m[1]); err == nil {
			meta.DeclaredTotalARS = v
		}
	}

	lines := strings.Split(text, "\n")

	var txns []parser.ParsedTransaction
	inDetail := false
	skipHeaders := 0
	var fields []string

	finalize := func() {
		if len(fields) >= 4 {
			if txn := buildTransaction(fields); txn != nil {
				txns = append(txns, *txn)
			}
		}
		fields = nil
	}

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)

		// Page header: finalize current transaction and leave detail mode to skip boilerplate.
		if strings.HasPrefix(line, "Resumen N") {
			finalize()
			inDetail = false
			continue
		}

		// Enter detail mode when we see the section header.
		if strings.HasPrefix(line, "DETALLE DEL CONSUMO") {
			inDetail = true
			skipHeaders = 5 // FECHA, REFERENCIA, COMPROBANTE, PESOS, DÓLARES
			continue
		}

		if !inDetail {
			continue
		}

		// Skip the 5 column headers following "DETALLE DEL CONSUMO".
		if skipHeaders > 0 {
			if line != "" {
				skipHeaders--
			}
			continue
		}

		if line == "" {
			continue
		}

		// End of transactions in this detail block.
		if strings.HasPrefix(line, "SUBTOTAL") || strings.HasPrefix(line, "TOTAL A PAGAR") {
			finalize()
			inDetail = false
			continue
		}

		// Section labels separating purchase groups — skip them.
		upper := strings.ToUpper(line)
		if upper == "COMPRAS DEL MES" || strings.HasPrefix(upper, "CUOTA") && strings.HasSuffix(upper, "DEL MES") {
			continue
		}

		// A date line starts a new transaction block.
		if reDateLine.MatchString(line) {
			finalize()
			fields = []string{line}
			continue
		}

		// Accumulate fields for the current transaction.
		if fields != nil {
			fields = append(fields, line)

			// Auto-finalize: a transaction has exactly 4 fields —
			// date, description, comprobante, amount.
			if len(fields) == 4 {
				finalize()
			}
		}
	}
	finalize()

	return meta, txns, nil
}

// buildTransaction converts a slice of raw text fields into a ParsedTransaction.
//
// Field layout:
//
//	fields[0]  date         "DD-Mmm-YY"
//	fields[1]  description  (may end with " N/M" for installments, e.g. "PSA   06/12")
//	fields[2]  comprobante  (5-digit number)
//	fields[3]  amount       Argentine format ("88.545,83")
func buildTransaction(fields []string) *parser.ParsedTransaction {
	if len(fields) < 4 {
		return nil
	}

	date, err := parseDate(fields[0])
	if err != nil {
		return nil
	}

	amt, err := parseAmount(fields[3])
	if err != nil {
		return nil
	}

	desc := fields[1]
	var isInstallment bool
	var cuotaCurrent, cuotaTotal int

	if m := reCuotaInDesc.FindStringSubmatch(desc); m != nil {
		isInstallment = true
		desc = strings.TrimSpace(m[1])
		cuotaCurrent, _ = strconv.Atoi(m[2])
		cuotaTotal, _ = strconv.Atoi(m[3])
	}

	currency := "ARS"
	if strings.Contains(desc, "USD") {
		currency = "USD"
	}

	return &parser.ParsedTransaction{
		Date:                date,
		DescriptionOriginal: desc,
		Amount:              math.Abs(amt),
		Currency:            currency,
		IsDebit:             amt >= 0,
		IsInstallment:       isInstallment,
		InstallmentCurrent:  cuotaCurrent,
		InstallmentTotal:    cuotaTotal,
		RawLine:             strings.Join(fields, " | "),
	}
}
