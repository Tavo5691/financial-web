// Package galicia_visa parses Visa Galicia credit card statements.
package galicia_visa

import (
	"financial-web/internal/parser"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func init() {
	parser.Register(&galiciaVisaParser{})
}

type galiciaVisaParser struct{}

func (p *galiciaVisaParser) ID() string { return "galicia_visa" }

func (p *galiciaVisaParser) Info() parser.ParserInfo {
	return parser.ParserInfo{
		ID:          "galicia_visa",
		Bank:        "Galicia",
		CardNetwork: "Visa",
		Description: "Resumen Visa Galicia — formato digital",
	}
}

var (
	// reDateLine matches transaction date lines: DD-MM-YY (two-digit year)
	reDateLine = regexp.MustCompile(`^(\d{2})-(\d{2})-(\d{2})$`)
	// reCuota matches installment notation: N/M (e.g. "08/09")
	reCuota = regexp.MustCompile(`^(\d{1,2})/(\d{2})$`)
	// rePeriod extracts YYYYMM from the statement ID line (e.g. "20260319074422179H")
	rePeriod = regexp.MustCompile(`(\d{4})(\d{2})\d{2}\d+H`)
	// reTotalPagar finds the declared total to pay
	reTotalPagar = regexp.MustCompile(`(?m)TOTAL A PAGAR\s*\n\s*([\d.]+,\d+)`)
)

// parseAmount converts Argentine number format ("1.234,56") to float64.
func parseAmount(s string) (float64, error) {
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	return strconv.ParseFloat(s, 64)
}

func (p *galiciaVisaParser) Parse(text string) (parser.StatementMeta, []parser.ParsedTransaction, error) {
	meta := parser.StatementMeta{
		CardType: "VISA",
		CardBank: "GALICIA",
	}

	// Extract period from statement ID line like "20260319074422179H" → "2026-03"
	if m := rePeriod.FindStringSubmatch(text); m != nil {
		meta.StatementPeriod = m[1] + "-" + m[2]
	}

	// Extract declared total from "TOTAL A PAGAR\n2.146.366,35"
	if m := reTotalPagar.FindStringSubmatch(text); m != nil {
		if v, err := parseAmount(m[1]); err == nil {
			meta.DeclaredTotalARS = v
		}
	}

	lines := strings.Split(text, "\n")

	var txns []parser.ParsedTransaction
	inDetail := false
	skipHeaders := 0
	skipAmounts := 0 // lines to skip after a card subtotal marker
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

		// Page header start: temporarily leave detail mode to skip boilerplate.
		// Transactions are always complete before page breaks so finalize is a no-op here.
		if strings.HasPrefix(line, "Resumen N") {
			finalize()
			inDetail = false
			continue
		}

		// Enter detail mode when we see the section header.
		if strings.HasPrefix(line, "DETALLE DEL CONSUMO") {
			inDetail = true
			skipHeaders = 6 // FECHA, REFERENCIA, CUOTA, COMPROBANTE, PESOS, DÓLARES
			continue
		}

		if !inDetail {
			continue
		}

		// Skip the 6 column headers following "DETALLE DEL CONSUMO".
		if skipHeaders > 0 {
			if line != "" {
				skipHeaders--
			}
			continue
		}

		// Skip subtotal amounts after a card summary line.
		if skipAmounts > 0 {
			if line != "" {
				skipAmounts--
			}
			continue
		}

		// End of all transactions.
		if strings.Contains(line, "TOTAL A PAGAR") || strings.HasPrefix(line, "Plan V:") {
			finalize()
			inDetail = false
			continue
		}

		// Card subtotal (e.g. "Total Consumos de  4546 4200 2666 8833"): finalize current
		// transaction and skip the two following totals (ARS + USD).
		if strings.Contains(line, "Total Consumos") {
			finalize()
			skipAmounts = 2
			// If this also starts with TARJETA it ends the detail section entirely.
			if strings.HasPrefix(line, "TARJETA") {
				inDetail = false
			}
			continue
		}

		if line == "" {
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

			// Auto-finalize once we have enough fields for a complete transaction.
			// Two layouts exist:
			//   standard (fields[1] is a 1-2 char reference like "*" or "K"):
			//     5 fields: date, ref, desc, comprobante, amount
			//     6 fields: date, ref, desc, cuota, comprobante, amount
			//   no-reference (fields[1] is the description — empty ref line was skipped):
			//     4 fields: date, desc, comprobante, amount
			//     5 fields: date, desc, cuota, comprobante, amount
			complete := false
			if len(fields) >= 2 {
				hasRef := len(fields[1]) <= 2
				if hasRef {
					switch len(fields) {
					case 5:
						complete = !reCuota.MatchString(fields[3])
					case 6:
						complete = true
					}
				} else {
					switch len(fields) {
					case 4:
						complete = !reCuota.MatchString(fields[2])
					case 5:
						complete = true
					}
				}
			}
			if complete {
				finalize()
			}
		}
	}
	finalize()

	return meta, txns, nil
}

// buildTransaction converts a slice of raw text fields into a ParsedTransaction.
//
// Two layouts are supported:
//
//	Standard (fields[1] is a short reference like "*" or "K"):
//	  fields[0]  date         "DD-MM-YY"
//	  fields[1]  reference    "*" or "K"
//	  fields[2]  description
//	  fields[3]  cuota (N/M) or comprobante
//	  fields[4]  comprobante (if cuota) or amount
//	  fields[5]  amount (if cuota)
//
//	No-reference (empty ref line was skipped; fields[1] is the description):
//	  fields[0]  date         "DD-MM-YY"
//	  fields[1]  description  (includes "USD N,NN" for foreign-currency transactions)
//	  fields[2]  cuota (N/M) or comprobante
//	  fields[3]  comprobante (if cuota) or amount
//	  fields[4]  amount (if cuota)
func buildTransaction(fields []string) *parser.ParsedTransaction {
	if len(fields) < 4 {
		return nil
	}

	date, err := time.Parse("02-01-06", fields[0])
	if err != nil {
		return nil
	}

	var desc, amtStr string
	var isInstallment bool
	var cuotaCurrent, cuotaTotal int

	hasRef := len(fields[1]) <= 2 // "*" or "K" vs a description

	if hasRef {
		if len(fields) < 5 {
			return nil
		}
		desc = fields[2]
		if m := reCuota.FindStringSubmatch(fields[3]); m != nil {
			if len(fields) < 6 {
				return nil
			}
			isInstallment = true
			cuotaCurrent, _ = strconv.Atoi(m[1])
			cuotaTotal, _ = strconv.Atoi(m[2])
			amtStr = fields[5]
		} else {
			amtStr = fields[4]
		}
	} else {
		desc = fields[1]
		if m := reCuota.FindStringSubmatch(fields[2]); m != nil {
			if len(fields) < 5 {
				return nil
			}
			isInstallment = true
			cuotaCurrent, _ = strconv.Atoi(m[1])
			cuotaTotal, _ = strconv.Atoi(m[2])
			amtStr = fields[4]
		} else {
			amtStr = fields[3]
		}
	}

	amt, err := parseAmount(amtStr)
	if err != nil {
		return nil
	}

	// USD transactions embed "USD" in the description (e.g. "WL *STEAM PURCHASE  USD  29,67").
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
