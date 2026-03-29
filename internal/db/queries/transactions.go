package queries

import (
	"database/sql"
	"financial-web/internal/domain"
	"fmt"
	"strings"
	"time"
)

// TransactionFilter holds optional filter parameters.
type TransactionFilter struct {
	Period      string // YYYY-MM, empty = all
	Card        string // VISA | MASTERCARD, empty = all
	CardBank    string // empty = all
	Category    string // empty = all
	Search      string // free text search in description_original / merchant
	ExpenseType string // "fixed" | "variable" | "installment", empty = all
	Hidden      bool   // if true, include hidden transactions
}

// ListTransactions returns transactions matching the given filter.
func ListTransactions(db *sql.DB, f TransactionFilter) ([]*domain.Transaction, error) {
	where := []string{}
	args := []any{}

	if !f.Hidden {
		where = append(where, "is_hidden = 0")
	}
	if f.Period != "" {
		where = append(where, "statement_period = ?")
		args = append(args, f.Period)
	}
	if f.Card != "" {
		where = append(where, "card_type = ?")
		args = append(args, f.Card)
	}
	if f.CardBank != "" {
		where = append(where, "card_bank = ?")
		args = append(args, f.CardBank)
	}
	if f.Category != "" {
		where = append(where, "category = ?")
		args = append(args, f.Category)
	}
	if f.ExpenseType != "" {
		where = append(where, "expense_type = ?")
		args = append(args, f.ExpenseType)
	}
	if f.Search != "" {
		where = append(where, "(UPPER(description_original) LIKE ? OR UPPER(merchant) LIKE ?)")
		q := "%" + strings.ToUpper(f.Search) + "%"
		args = append(args, q, q)
	}

	clause := ""
	if len(where) > 0 {
		clause = "WHERE " + strings.Join(where, " AND ")
	}

	rows, err := db.Query(`
		SELECT id, statement_id, statement_period, card_type, card_bank,
		       transaction_date, description_original, merchant, alias_id,
		       category, amount, currency, amount_ars, exchange_rate,
		       is_installment, installment_current, installment_total,
		       installment_group_id, installment_original_amount,
		       expense_type, is_debit, is_hidden, notes, created_at, updated_at
		FROM transactions `+clause+`
		ORDER BY transaction_date DESC, id ASC`, args...)
	if err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}
	defer rows.Close()
	return scanTransactions(rows)
}

// ListByStatement returns all transactions for a given statement, ordered by date.
func ListByStatement(db *sql.DB, statementID int64) ([]*domain.Transaction, error) {
	rows, err := db.Query(`
		SELECT id, statement_id, statement_period, card_type, card_bank,
		       transaction_date, description_original, merchant, alias_id,
		       category, amount, currency, amount_ars, exchange_rate,
		       is_installment, installment_current, installment_total,
		       installment_group_id, installment_original_amount,
		       expense_type, is_debit, is_hidden, notes, created_at, updated_at
		FROM transactions
		WHERE statement_id = ?
		ORDER BY transaction_date ASC, id ASC`, statementID)
	if err != nil {
		return nil, fmt.Errorf("list by statement: %w", err)
	}
	defer rows.Close()
	return scanTransactions(rows)
}

// GetTransactionByID returns a single transaction.
func GetTransactionByID(db *sql.DB, id string) (*domain.Transaction, error) {
	rows, err := db.Query(`
		SELECT id, statement_id, statement_period, card_type, card_bank,
		       transaction_date, description_original, merchant, alias_id,
		       category, amount, currency, amount_ars, exchange_rate,
		       is_installment, installment_current, installment_total,
		       installment_group_id, installment_original_amount,
		       expense_type, is_debit, is_hidden, notes, created_at, updated_at
		FROM transactions WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	txns, err := scanTransactions(rows)
	if err != nil {
		return nil, err
	}
	if len(txns) == 0 {
		return nil, fmt.Errorf("transaction %q not found", id)
	}
	return txns[0], nil
}

// Execer is implemented by both *sql.DB and *sql.Tx, allowing InsertTransaction
// to be called either outside or inside a database transaction.
type Execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// InsertTransaction inserts a new transaction row.
func InsertTransaction(db Execer, t *domain.Transaction) error {
	var exchangeRate sql.NullFloat64
	if t.ExchangeRate != nil {
		exchangeRate = sql.NullFloat64{Float64: *t.ExchangeRate, Valid: true}
	}
	var instOriginal sql.NullFloat64
	if t.InstallmentOriginalAmount != nil {
		instOriginal = sql.NullFloat64{Float64: *t.InstallmentOriginalAmount, Valid: true}
	}
	var aliasID sql.NullInt64
	if t.AliasID != nil {
		aliasID = sql.NullInt64{Int64: *t.AliasID, Valid: true}
	}
	dateStr := t.TransactionDate.Format("2006-01-02")

	_, err := db.Exec(`
		INSERT INTO transactions (
			id, statement_id, statement_period, card_type, card_bank,
			transaction_date, description_original, merchant, alias_id,
			category, amount, currency, amount_ars, exchange_rate,
			is_installment, installment_current, installment_total,
			installment_group_id, installment_original_amount,
			expense_type, is_debit, is_hidden, notes
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.StatementID, t.StatementPeriod, t.CardType, t.CardBank,
		dateStr, t.DescriptionOriginal, t.Merchant, aliasID,
		t.Category, t.Amount, t.Currency, t.AmountARS, exchangeRate,
		boolToInt(t.IsInstallment), t.InstallmentCurrent, t.InstallmentTotal,
		t.InstallmentGroupID, instOriginal,
		t.ExpenseType, boolToInt(t.IsDebit), boolToInt(t.IsHidden), t.Notes,
	)
	return err
}

// UpdateTransaction updates editable fields on a transaction.
func UpdateTransaction(db *sql.DB, id string, merchant, category, expenseType, notes string) error {
	_, err := db.Exec(`
		UPDATE transactions SET merchant = ?, category = ?, expense_type = ?,
		                        notes = ?, updated_at = ?
		WHERE id = ?`, merchant, category, expenseType, notes, time.Now(), id)
	return err
}

// ToggleHidden flips the is_hidden flag and returns the new value.
func ToggleHidden(db *sql.DB, id string) (bool, error) {
	_, err := db.Exec(`
		UPDATE transactions SET is_hidden = 1 - is_hidden, updated_at = ?
		WHERE id = ?`, time.Now(), id)
	if err != nil {
		return false, err
	}
	var hidden int
	err = db.QueryRow(`SELECT is_hidden FROM transactions WHERE id = ?`, id).Scan(&hidden)
	return hidden == 1, err
}

// NextID returns the next available transaction ID for a given period and card type.
// Format: YYYY-MM-CARDTYPE-NNN
func NextID(db *sql.DB, period, cardType string) (string, error) {
	var maxSeq sql.NullInt64
	err := db.QueryRow(`
		SELECT MAX(CAST(SUBSTR(id, -3) AS INTEGER))
		FROM transactions
		WHERE statement_period = ? AND card_type = ?`, period, cardType).Scan(&maxSeq)
	if err != nil {
		return "", err
	}
	next := 1
	if maxSeq.Valid {
		next = int(maxSeq.Int64) + 1
	}
	return fmt.Sprintf("%s-%s-%03d", period, cardType, next), nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func scanTransactions(rows *sql.Rows) ([]*domain.Transaction, error) {
	var result []*domain.Transaction
	for rows.Next() {
		t := &domain.Transaction{}
		var aliasID sql.NullInt64
		var exchangeRate sql.NullFloat64
		var instOriginal sql.NullFloat64
		var dateStr string

		err := rows.Scan(
			&t.ID, &t.StatementID, &t.StatementPeriod, &t.CardType, &t.CardBank,
			&dateStr, &t.DescriptionOriginal, &t.Merchant, &aliasID,
			&t.Category, &t.Amount, &t.Currency, &t.AmountARS, &exchangeRate,
			&t.IsInstallment, &t.InstallmentCurrent, &t.InstallmentTotal,
			&t.InstallmentGroupID, &instOriginal,
			&t.ExpenseType, &t.IsDebit, &t.IsHidden, &t.Notes,
			&t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}

		t.TransactionDate, _ = time.Parse("2006-01-02", dateStr)

		if aliasID.Valid {
			t.AliasID = &aliasID.Int64
		}
		if exchangeRate.Valid {
			t.ExchangeRate = &exchangeRate.Float64
		}
		if instOriginal.Valid {
			t.InstallmentOriginalAmount = &instOriginal.Float64
		}

		result = append(result, t)
	}
	return result, rows.Err()
}
