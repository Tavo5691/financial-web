// migrate imports existing CSV data into the new SQLite database.
// Run once after the database is freshly created.
//
// Usage:
//
//	go run ./cmd/migrate \
//	  --transactions path/to/transactions.csv \
//	  --statements   path/to/statements_log.csv \
//	  --merchants    path/to/merchants.csv \
//	  --categories   path/to/categories.csv \
//	  --db           ./data/financial.db
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"financial-web/internal/db"
)

func main() {
	txnsPath := flag.String("transactions", "", "path to transactions.csv (required)")
	stmtsPath := flag.String("statements", "", "path to statements_log.csv (required)")
	merchantsPath := flag.String("merchants", "", "path to merchants.csv (required)")
	categoriesPath := flag.String("categories", "", "path to categories.csv (required)")
	dbPath := flag.String("db", "./data/financial.db", "path to SQLite database")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	if *txnsPath == "" || *stmtsPath == "" || *merchantsPath == "" || *categoriesPath == "" {
		fmt.Fprintln(os.Stderr, "error: all four CSV paths are required")
		flag.Usage()
		os.Exit(1)
	}

	database, err := db.Open(*dbPath)
	if err != nil {
		logger.Error("open db", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// 1. Migrate categories (update keywords column; rows already seeded by migration).
	logger.Info("migrating categories...")
	if err := migrateCategories(database, *categoriesPath, logger); err != nil {
		logger.Error("categories", "error", err)
		os.Exit(1)
	}

	// 2. Migrate merchants → aliases.
	logger.Info("migrating merchants → aliases...")
	if err := migrateMerchants(database, *merchantsPath, logger); err != nil {
		logger.Error("merchants", "error", err)
		os.Exit(1)
	}

	// 3. Migrate statements_log → statements.
	logger.Info("migrating statements...")
	stmtIDByFile, err := migrateStatements(database, *stmtsPath, logger)
	if err != nil {
		logger.Error("statements", "error", err)
		os.Exit(1)
	}

	// 4. Migrate transactions.
	logger.Info("migrating transactions...")
	if err := migrateTransactions(database, *txnsPath, stmtIDByFile, logger); err != nil {
		logger.Error("transactions", "error", err)
		os.Exit(1)
	}

	logger.Info("migration complete")
}

// migrateCategories updates keyword columns from the CSV.
func migrateCategories(database *db.DB, path string, logger *slog.Logger) error {
	records, err := readCSV(path)
	if err != nil {
		return err
	}
	headers := records[0]
	idxCategory := mustIdx(headers, "category")
	idxKeywords := mustIdx(headers, "keywords")
	idxExpenseType := mustIdx(headers, "default_expense_type")

	count := 0
	for _, row := range records[1:] {
		name := row[idxCategory]
		keywords := row[idxKeywords]
		expType := row[idxExpenseType]
		_, err := database.Exec(`
			INSERT INTO categories (name, default_expense_type, keywords)
			VALUES (?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET keywords = excluded.keywords, default_expense_type = excluded.default_expense_type`,
			name, expType, keywords)
		if err != nil {
			logger.Warn("insert/update category", "name", name, "error", err)
			continue
		}
		count++
	}
	logger.Info("categories done", "count", count)
	return nil
}

// migrateMerchants imports merchants.csv as aliases.
func migrateMerchants(database *db.DB, path string, logger *slog.Logger) error {
	records, err := readCSV(path)
	if err != nil {
		return err
	}
	headers := records[0]
	idxPattern := mustIdx(headers, "description_pattern")
	idxMerchant := mustIdx(headers, "merchant")
	idxCategory := mustIdx(headers, "category")

	count := 0
	for _, row := range records[1:] {
		pattern := strings.ToUpper(strings.TrimSpace(row[idxPattern]))
		merchant := strings.TrimSpace(row[idxMerchant])
		category := strings.TrimSpace(row[idxCategory])

		if pattern == "" {
			continue
		}

		// Look up default_expense_type from the category.
		var expType string
		err := database.QueryRow(`SELECT default_expense_type FROM categories WHERE name = ?`, category).Scan(&expType)
		if err != nil || expType == "" {
			expType = "variable"
		}

		_, err = database.Exec(`
			INSERT INTO aliases (pattern, merchant, category, expense_type)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(pattern) DO UPDATE SET
				merchant = excluded.merchant,
				category = excluded.category,
				expense_type = excluded.expense_type,
				updated_at = CURRENT_TIMESTAMP`,
			pattern, merchant, category, expType)
		if err != nil {
			logger.Warn("insert alias", "pattern", pattern, "error", err)
			continue
		}
		count++
	}
	logger.Info("aliases done", "count", count)
	return nil
}

// migrateStatements imports statements_log.csv and returns a map of source_file → id.
func migrateStatements(database *db.DB, path string, logger *slog.Logger) (map[string]int64, error) {
	records, err := readCSV(path)
	if err != nil {
		return nil, err
	}
	headers := records[0]
	idxFile := mustIdx(headers, "source_file")
	idxCardType := mustIdx(headers, "card_type")
	idxCardBank := mustIdx(headers, "card_bank")
	idxPeriod := mustIdx(headers, "statement_period")
	idxTotal := mustIdx(headers, "total_transactions")
	idxAmount := mustIdx(headers, "total_amount_ars")
	idxRate := mustIdx(headers, "exchange_rate_usd")

	idByFile := map[string]int64{}
	count := 0
	for _, row := range records[1:] {
		sourceFile := strings.TrimSpace(row[idxFile])
		cardType := strings.TrimSpace(row[idxCardType])
		cardBank := strings.TrimSpace(row[idxCardBank])
		period := strings.TrimSpace(row[idxPeriod])
		totalTxns, _ := strconv.Atoi(strings.TrimSpace(row[idxTotal]))
		totalARS, _ := strconv.ParseFloat(strings.TrimSpace(row[idxAmount]), 64)

		var ratePtr *float64
		if rateStr := strings.TrimSpace(row[idxRate]); rateStr != "" {
			r, _ := strconv.ParseFloat(rateStr, 64)
			ratePtr = &r
		}

		// Use a synthetic hash (file path) since we have no actual file to hash.
		hash := fmt.Sprintf("migrated:%s", sourceFile)

		res, err := database.Exec(`
			INSERT OR IGNORE INTO statements
			  (source_file, file_hash, card_type, card_bank, statement_period,
			   parser_id, status, processed_at, total_txns, total_amount_ars, exchange_rate_usd)
			VALUES (?, ?, ?, ?, ?, '', 'reviewed', CURRENT_TIMESTAMP, ?, ?, ?)`,
			sourceFile, hash, cardType, cardBank, period, totalTxns, totalARS, ratePtr)
		if err != nil {
			logger.Warn("insert statement", "file", sourceFile, "error", err)
			continue
		}

		id, _ := res.LastInsertId()
		if id == 0 {
			// Already existed — fetch its id.
			database.QueryRow(`SELECT id FROM statements WHERE source_file = ?`, sourceFile).Scan(&id)
		}
		idByFile[sourceFile] = id
		count++
	}
	logger.Info("statements done", "count", count)
	return idByFile, nil
}

// migrateTransactions imports transactions.csv.
func migrateTransactions(database *db.DB, path string, stmtIDByFile map[string]int64, logger *slog.Logger) error {
	records, err := readCSV(path)
	if err != nil {
		return err
	}
	headers := records[0]

	idx := func(name string) int { return mustIdx(headers, name) }

	count, skipped := 0, 0
	for i, row := range records[1:] {
		sourceFile := strings.TrimSpace(row[idx("source_file")])
		stmtID, ok := stmtIDByFile[sourceFile]
		if !ok {
			// Statement not in log — create a placeholder.
			hash := fmt.Sprintf("migrated:orphan:%s", sourceFile)
			res, err := database.Exec(`
				INSERT OR IGNORE INTO statements
				  (source_file, file_hash, card_type, card_bank, statement_period, status)
				VALUES (?, ?, '', '', '', 'reviewed')`, sourceFile, hash)
			if err == nil {
				stmtID, _ = res.LastInsertId()
				if stmtID == 0 {
					database.QueryRow(`SELECT id FROM statements WHERE source_file = ?`, sourceFile).Scan(&stmtID)
				}
				stmtIDByFile[sourceFile] = stmtID
			}
		}

		txnID := strings.TrimSpace(row[idx("id")])
		if txnID == "" {
			logger.Warn("skipping row with empty id", "row", i+2)
			skipped++
			continue
		}

		period := strings.TrimSpace(row[idx("statement_period")])
		cardType := strings.TrimSpace(row[idx("card_type")])
		cardBank := strings.TrimSpace(row[idx("card_bank")])
		date := strings.TrimSpace(row[idx("transaction_date")])
		descOrig := strings.TrimSpace(row[idx("description_original")])
		merchant := strings.TrimSpace(row[idx("merchant")])
		category := strings.TrimSpace(row[idx("category")])
		amount, _ := strconv.ParseFloat(strings.TrimSpace(row[idx("amount")]), 64)
		currency := strings.TrimSpace(row[idx("currency")])
		amountARS, _ := strconv.ParseFloat(strings.TrimSpace(row[idx("amount_ars")]), 64)
		expenseType := strings.TrimSpace(row[idx("expense_type")])
		notes := strings.TrimSpace(row[idx("notes")])

		isInstallment := parseBool(row[idx("is_installment")])
		isDebit := parseBool(row[idx("is_debit")])

		instCurrent, _ := strconv.Atoi(strings.TrimSpace(row[idx("installment_current")]))
		instTotal, _ := strconv.Atoi(strings.TrimSpace(row[idx("installment_total")]))
		instGroupID := strings.TrimSpace(row[idx("installment_group_id")])

		var exchangeRate *float64
		if rateStr := strings.TrimSpace(row[idx("exchange_rate")]); rateStr != "" {
			r, _ := strconv.ParseFloat(rateStr, 64)
			exchangeRate = &r
		}
		var instOriginal *float64
		if origStr := strings.TrimSpace(row[idx("installment_original_amount")]); origStr != "" {
			o, _ := strconv.ParseFloat(origStr, 64)
			instOriginal = &o
		}

		_, err := database.Exec(`
			INSERT OR IGNORE INTO transactions (
				id, statement_id, statement_period, card_type, card_bank,
				transaction_date, description_original, merchant,
				category, amount, currency, amount_ars, exchange_rate,
				is_installment, installment_current, installment_total,
				installment_group_id, installment_original_amount,
				expense_type, is_debit, notes
			) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			txnID, stmtID, period, cardType, cardBank,
			date, descOrig, merchant,
			category, amount, currency, amountARS, exchangeRate,
			boolInt(isInstallment), instCurrent, instTotal,
			instGroupID, instOriginal,
			expenseType, boolInt(isDebit), notes,
		)
		if err != nil {
			logger.Warn("insert transaction", "id", txnID, "error", err)
			skipped++
			continue
		}
		count++
	}
	logger.Info("transactions done", "imported", count, "skipped", skipped)
	return nil
}

// --- helpers ---

func readCSV(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.LazyQuotes = true
	return r.ReadAll()
}

func mustIdx(headers []string, name string) int {
	for i, h := range headers {
		if strings.TrimSpace(h) == name {
			return i
		}
	}
	panic(fmt.Sprintf("column %q not found in headers: %v", name, headers))
}

func parseBool(s string) bool {
	return strings.EqualFold(strings.TrimSpace(s), "true")
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Unused but keeps the import.
var _ = time.Now
