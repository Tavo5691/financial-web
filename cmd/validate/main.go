// validate is a CLI tool for developing and testing bank statement parsers.
//
// Usage:
//
//	go run ./cmd/validate -list
//	go run ./cmd/validate -extract <pdf>
//	go run ./cmd/validate [-expected N] [-verbose] <parser-id> <pdf>
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"financial-web/internal/parser"
	_ "financial-web/internal/parser/bbva_visa"
	_ "financial-web/internal/parser/galicia_mastercard"
	_ "financial-web/internal/parser/galicia_visa"
	_ "financial-web/internal/parser/uala_mastercard"
	"financial-web/internal/pdf"
)

func main() {
	list := flag.Bool("list", false, "list all registered parsers and exit")
	extract := flag.Bool("extract", false, "extract raw text from PDF and print to stdout")
	expected := flag.Int("expected", 0, "expected number of transactions (0 = skip check)")
	verbose := flag.Bool("verbose", false, "print all transactions, not just the first 5")
	flag.Usage = usage
	flag.Parse()

	if *list {
		printParsers()
		os.Exit(0)
	}

	args := flag.Args()

	// -extract mode: just one arg (the pdf path)
	if *extract {
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "error: -extract requires a PDF path")
			os.Exit(1)
		}
		text, err := pdf.ExtractText(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(text)
		os.Exit(0)
	}

	// Normal mode: <parser-id> <pdf>
	if len(args) < 2 {
		usage()
		os.Exit(1)
	}
	parserID := args[0]
	pdfPath := args[1]

	p, ok := parser.Get(parserID)
	if !ok {
		fmt.Fprintf(os.Stderr, "error: parser %q not found. Run with -list to see available parsers.\n", parserID)
		os.Exit(1)
	}

	fmt.Printf("\n%s\n", strings.Repeat("=", 60))
	fmt.Printf("  VALIDATE: %s\n", p.Info().ID)
	fmt.Printf("  PDF: %s\n", pdfPath)
	fmt.Printf("%s\n\n", strings.Repeat("=", 60))

	var meta parser.StatementMeta
	var txns []parser.ParsedTransaction
	var parseErr error

	if fp, ok := p.(parser.FileParser); ok {
		meta, txns, parseErr = fp.ParseFile(pdfPath)
	} else {
		text, err := pdf.ExtractText(pdfPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  error extracting PDF text: %v\n", err)
			os.Exit(1)
		}
		meta, txns, parseErr = p.Parse(text)
	}

	fmt.Println("  METADATA")
	fmt.Println("  " + strings.Repeat("-", 40))
	fmt.Printf("  Bank / Card:    %s / %s\n", meta.CardBank, meta.CardType)
	fmt.Printf("  Period:         %s\n", meta.StatementPeriod)
	if meta.ExchangeRateUSD > 0 {
		fmt.Printf("  Exchange rate:  ARS %.2f / USD\n", meta.ExchangeRateUSD)
	}
	if meta.DeclaredTotalARS > 0 {
		fmt.Printf("  Declared total: ARS $%.2f\n", meta.DeclaredTotalARS)
	}

	fmt.Println("\n  RESULTS")
	fmt.Println("  " + strings.Repeat("-", 40))

	pass := true
	if *expected > 0 {
		status := "PASS"
		if len(txns) != *expected {
			status = "FAIL"
			pass = false
		}
		fmt.Printf("  Transactions found: %d / expected %d   [%s]\n", len(txns), *expected, status)
	} else {
		fmt.Printf("  Transactions found: %d\n", len(txns))
	}

	if parseErr != nil {
		fmt.Printf("  Parse error:        %v\n", parseErr)
		pass = false
	} else {
		fmt.Printf("  Parse errors:       0\n")
	}

	// Count installment groups
	groups := map[string]struct{}{}
	for _, t := range txns {
		if t.IsInstallment && t.InstallmentCurrent == 1 {
			groups[t.DescriptionOriginal] = struct{}{}
		}
	}
	fmt.Printf("  Installment groups: %d\n", len(groups))

	// Sample output
	limit := 5
	if *verbose {
		limit = len(txns)
	}
	if len(txns) > 0 {
		fmt.Println("\n  SAMPLE (first 5 transactions)")
		fmt.Println("  " + strings.Repeat("-", 40))
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  Date\tDescription\tAmount\tCurr\tInst")
		fmt.Fprintln(tw, "  ----------\t-----------------------\t----------\t----\t----")
		for i, t := range txns {
			if i >= limit {
				break
			}
			inst := ""
			if t.IsInstallment {
				inst = fmt.Sprintf("%d/%d", t.InstallmentCurrent, t.InstallmentTotal)
			}
			debit := ""
			if !t.IsDebit {
				debit = " (crédito)"
			}
			fmt.Fprintf(tw, "  %s\t%s%s\t%.2f\t%s\t%s\n",
				t.Date.Format("2006-01-02"),
				truncate(t.DescriptionOriginal, 40),
				debit,
				t.Amount,
				t.Currency,
				inst,
			)
		}
		tw.Flush()
		if !*verbose && len(txns) > limit {
			fmt.Printf("\n  … and %d more. Use -verbose to see all.\n", len(txns)-limit)
		}
	}

	fmt.Printf("\n%s\n", strings.Repeat("=", 60))
	if pass {
		fmt.Println("  STATUS: PASS")
	} else {
		fmt.Println("  STATUS: FAIL")
	}
	fmt.Printf("%s\n\n", strings.Repeat("=", 60))

	if !pass {
		os.Exit(1)
	}
}

func printParsers() {
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tBank\tCard\tDescription")
	fmt.Fprintln(tw, "--\t----\t----\t-----------")
	for _, p := range parser.All() {
		info := p.Info()
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", info.ID, info.Bank, info.CardNetwork, info.Description)
	}
	tw.Flush()
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  go run ./cmd/validate -list
  go run ./cmd/validate -extract <pdf-path>
  go run ./cmd/validate [-expected N] [-verbose] <parser-id> <pdf-path>

Flags:`)
	flag.PrintDefaults()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
