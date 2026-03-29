package queries

import (
	"database/sql"
	"financial-web/internal/domain"
	"fmt"
	"strings"
	"time"
)

// ListAliases returns all aliases ordered by pattern.
func ListAliases(db *sql.DB) ([]*domain.Alias, error) {
	rows, err := db.Query(`
		SELECT id, pattern, merchant, category, expense_type, notes, created_at, updated_at
		FROM aliases ORDER BY pattern ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAliases(rows)
}

// GetAliasByID returns a single alias.
func GetAliasByID(db *sql.DB, id int64) (*domain.Alias, error) {
	row := db.QueryRow(`
		SELECT id, pattern, merchant, category, expense_type, notes, created_at, updated_at
		FROM aliases WHERE id = ?`, id)
	return scanAlias(row)
}

// InsertAlias creates a new alias and returns its ID.
func InsertAlias(db *sql.DB, a *domain.Alias) (int64, error) {
	res, err := db.Exec(`
		INSERT INTO aliases (pattern, merchant, category, expense_type, notes)
		VALUES (?, ?, ?, ?, ?)`,
		strings.ToUpper(a.Pattern), a.Merchant, a.Category, a.ExpenseType, a.Notes)
	if err != nil {
		return 0, fmt.Errorf("insert alias: %w", err)
	}
	return res.LastInsertId()
}

// UpdateAlias updates an existing alias's fields.
func UpdateAlias(db *sql.DB, a *domain.Alias) error {
	_, err := db.Exec(`
		UPDATE aliases SET pattern = ?, merchant = ?, category = ?, expense_type = ?,
		                   notes = ?, updated_at = ?
		WHERE id = ?`,
		strings.ToUpper(a.Pattern), a.Merchant, a.Category, a.ExpenseType,
		a.Notes, time.Now(), a.ID)
	return err
}

// DeleteAlias removes an alias by ID.
func DeleteAlias(db *sql.DB, id int64) error {
	_, err := db.Exec(`DELETE FROM aliases WHERE id = ?`, id)
	return err
}

// ApplyAliasToTransactions propagates an alias to all matching transactions.
// Returns the number of rows updated.
func ApplyAliasToTransactions(db *sql.DB, aliasID int64, pattern, merchant, category, expenseType string) (int64, error) {
	res, err := db.Exec(`
		UPDATE transactions
		SET merchant     = ?,
		    category     = ?,
		    expense_type = ?,
		    alias_id     = ?,
		    updated_at   = CURRENT_TIMESTAMP
		WHERE is_hidden = 0
		  AND UPPER(description_original) LIKE '%' || UPPER(?) || '%'`,
		merchant, category, expenseType, aliasID, pattern)
	if err != nil {
		return 0, fmt.Errorf("apply alias: %w", err)
	}
	return res.RowsAffected()
}

// ListCategories returns all categories ordered by sort_order.
func ListCategories(db *sql.DB) ([]*domain.Category, error) {
	rows, err := db.Query(`SELECT name, default_expense_type, keywords, sort_order FROM categories ORDER BY sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cats []*domain.Category
	for rows.Next() {
		c := &domain.Category{}
		if err := rows.Scan(&c.Name, &c.DefaultExpenseType, &c.Keywords, &c.SortOrder); err != nil {
			return nil, err
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}

func scanAlias(row *sql.Row) (*domain.Alias, error) {
	a := &domain.Alias{}
	err := row.Scan(&a.ID, &a.Pattern, &a.Merchant, &a.Category, &a.ExpenseType, &a.Notes, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan alias: %w", err)
	}
	return a, nil
}

func scanAliases(rows *sql.Rows) ([]*domain.Alias, error) {
	var result []*domain.Alias
	for rows.Next() {
		a := &domain.Alias{}
		if err := rows.Scan(&a.ID, &a.Pattern, &a.Merchant, &a.Category, &a.ExpenseType, &a.Notes, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan alias row: %w", err)
		}
		result = append(result, a)
	}
	return result, rows.Err()
}
