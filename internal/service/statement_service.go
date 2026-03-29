package service

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"financial-web/internal/categorizer"
	"financial-web/internal/db/queries"
	"financial-web/internal/domain"
	"financial-web/internal/parser"
	"financial-web/internal/pdf"
)

// StatementService orchestrates PDF parsing and transaction storage.
type StatementService struct {
	DB           *sql.DB
	ProcessedDir string // if set, successfully processed PDFs are moved here
}

// ProcessResult summarises the outcome of processing a statement.
type ProcessResult struct {
	StatementID int64
	TotalTxns   int
	TotalARS    float64
	Period      string
	CardType    string
	CardBank    string
}

// Process parses the PDF at pdfPath using the named parser, categorizes all
// transactions, and persists them to the database.
func (s *StatementService) Process(statementID int64, parserID string, exchangeRateOverride float64) (*ProcessResult, error) {
	stmt, err := queries.GetStatementByID(s.DB, statementID)
	if err != nil {
		return nil, fmt.Errorf("get statement: %w", err)
	}

	p, ok := parser.Get(parserID)
	if !ok {
		return nil, fmt.Errorf("parser %q not registered", parserID)
	}

	var meta parser.StatementMeta
	var rawTxns []parser.ParsedTransaction
	var parseErr error

	if fp, ok := p.(parser.FileParser); ok {
		meta, rawTxns, parseErr = fp.ParseFile(stmt.SourceFile)
	} else {
		var text string
		text, err = pdf.ExtractText(stmt.SourceFile)
		if err != nil {
			_ = queries.UpdateError(s.DB, statementID, err.Error())
			return nil, fmt.Errorf("extract PDF: %w", err)
		}
		meta, rawTxns, parseErr = p.Parse(text)
	}
	if parseErr != nil {
		_ = queries.UpdateError(s.DB, statementID, parseErr.Error())
		return nil, fmt.Errorf("parse PDF: %w", parseErr)
	}

	// Resolve exchange rate: parser-provided > override > 0
	exchangeRate := meta.ExchangeRateUSD
	if exchangeRateOverride > 0 {
		exchangeRate = exchangeRateOverride
	}

	// Load aliases and categories for categorization.
	aliases, err := queries.ListAliases(s.DB)
	if err != nil {
		return nil, fmt.Errorf("load aliases: %w", err)
	}
	categories, err := queries.ListCategories(s.DB)
	if err != nil {
		return nil, fmt.Errorf("load categories: %w", err)
	}

	// Fetch the current max sequence number once, before the loop.
	var maxSeq int
	s.DB.QueryRow(`
		SELECT COALESCE(MAX(CAST(SUBSTR(id, -3) AS INTEGER)), 0)
		FROM transactions WHERE statement_period = ? AND card_type = ?`,
		meta.StatementPeriod, meta.CardType).Scan(&maxSeq)
	seq := maxSeq

	var totalARS float64
	var txns []*domain.Transaction

	for _, raw := range rawTxns {
		cat := categorizer.Categorize(s.DB, raw.DescriptionOriginal, aliases, categories)

		// Determine ARS amount.
		amountARS := raw.Amount
		if raw.Currency == domain.CurrencyUSD && exchangeRate > 0 {
			amountARS = raw.Amount * exchangeRate
		}

		// Generate ID by incrementing the local counter.
		seq++
		txnID := fmt.Sprintf("%s-%s-%03d", meta.StatementPeriod, meta.CardType, seq)

		var erPtr *float64
		if raw.Currency == domain.CurrencyUSD {
			er := exchangeRate
			erPtr = &er
		}
		var instOrig *float64
		if raw.IsInstallment && raw.InstallmentOriginalAmount > 0 {
			v := raw.InstallmentOriginalAmount
			instOrig = &v
		}

		t := &domain.Transaction{
			ID:                        txnID,
			StatementID:               statementID,
			StatementPeriod:           meta.StatementPeriod,
			CardType:                  meta.CardType,
			CardBank:                  meta.CardBank,
			TransactionDate:           raw.Date,
			DescriptionOriginal:       raw.DescriptionOriginal,
			Merchant:                  cat.Merchant,
			AliasID:                   cat.AliasID,
			Category:                  cat.Category,
			Amount:                    raw.Amount,
			Currency:                  raw.Currency,
			AmountARS:                 amountARS,
			ExchangeRate:              erPtr,
			IsInstallment:             raw.IsInstallment,
			InstallmentCurrent:        raw.InstallmentCurrent,
			InstallmentTotal:          raw.InstallmentTotal,
			InstallmentOriginalAmount: instOrig,
			ExpenseType:               cat.ExpenseType,
			IsDebit:                   raw.IsDebit,
		}

		if raw.IsInstallment && raw.InstallmentCurrent == 1 {
			t.InstallmentGroupID = fmt.Sprintf("INST-%s-%s-%s",
				meta.StatementPeriod, meta.CardType,
				sanitizeForID(raw.DescriptionOriginal))
		}

		txns = append(txns, t)
		if raw.IsDebit {
			totalARS += amountARS
		}
	}

	// Insert all transactions atomically using a real DB transaction.
	tx, err := s.DB.Begin()
	if err != nil {
		return nil, err
	}
	for _, t := range txns {
		if err := queries.InsertTransaction(tx, t); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("insert txn %s: %w", t.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	var erPtr *float64
	if exchangeRate > 0 {
		erPtr = &exchangeRate
	}
	if err := queries.UpdateProcessed(s.DB, statementID,
		meta.CardType, meta.CardBank, meta.StatementPeriod, parserID,
		len(txns), totalARS, erPtr); err != nil {
		return nil, fmt.Errorf("update statement: %w", err)
	}

	if s.ProcessedDir != "" {
		if err := moveToProcessed(stmt.SourceFile, s.ProcessedDir); err != nil {
			// Non-fatal: log but don't fail the operation.
			_ = err
		}
	}

	return &ProcessResult{
		StatementID: statementID,
		TotalTxns:   len(txns),
		TotalARS:    totalARS,
		Period:      meta.StatementPeriod,
		CardType:    meta.CardType,
		CardBank:    meta.CardBank,
	}, nil
}

// moveToProcessed moves src to destDir, creating destDir if needed.
// If a file with the same name already exists in destDir, the source is not moved.
func moveToProcessed(src, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(destDir, filepath.Base(src))
	if _, err := os.Stat(dst); err == nil {
		// Destination already exists — skip to avoid overwriting.
		return nil
	}
	return os.Rename(src, dst)
}

// sanitizeForID strips non-alphanumeric characters for use in IDs.
func sanitizeForID(s string) string {
	var out []rune
	for _, c := range s {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			out = append(out, c)
		}
	}
	if len(out) > 20 {
		out = out[:20]
	}
	return string(out)
}

