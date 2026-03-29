package queries

import (
	"database/sql"
	"financial-web/internal/domain"
	"fmt"
)

// GetCategoryByName returns a single category by name.
func GetCategoryByName(db *sql.DB, name string) (*domain.Category, error) {
	c := &domain.Category{}
	err := db.QueryRow(
		`SELECT name, default_expense_type, keywords, sort_order FROM categories WHERE name = ?`, name,
	).Scan(&c.Name, &c.DefaultExpenseType, &c.Keywords, &c.SortOrder)
	if err != nil {
		return nil, fmt.Errorf("get category: %w", err)
	}
	return c, nil
}

// InsertCategory creates a new category.
func InsertCategory(db *sql.DB, c *domain.Category) error {
	_, err := db.Exec(
		`INSERT INTO categories (name, default_expense_type, keywords, sort_order) VALUES (?, ?, ?, ?)`,
		c.Name, c.DefaultExpenseType, c.Keywords, c.SortOrder,
	)
	if err != nil {
		return fmt.Errorf("insert category: %w", err)
	}
	return nil
}

// UpdateCategory updates a category. If the name changed, cascades to aliases and transactions.
func UpdateCategory(db *sql.DB, oldName string, c *domain.Category) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`UPDATE categories SET name = ?, default_expense_type = ?, keywords = ?, sort_order = ? WHERE name = ?`,
		c.Name, c.DefaultExpenseType, c.Keywords, c.SortOrder, oldName,
	)
	if err != nil {
		return fmt.Errorf("update category: %w", err)
	}

	if c.Name != oldName {
		if _, err = tx.Exec(`UPDATE aliases SET category = ? WHERE category = ?`, c.Name, oldName); err != nil {
			return fmt.Errorf("cascade alias category: %w", err)
		}
		if _, err = tx.Exec(`UPDATE transactions SET category = ? WHERE category = ?`, c.Name, oldName); err != nil {
			return fmt.Errorf("cascade transaction category: %w", err)
		}
	}

	return tx.Commit()
}

// DeleteCategory removes a category by name.
func DeleteCategory(db *sql.DB, name string) error {
	_, err := db.Exec(`DELETE FROM categories WHERE name = ?`, name)
	return err
}

// CountCategoryUsage returns how many aliases and transactions reference this category.
func CountCategoryUsage(db *sql.DB, name string) (aliases, transactions int, err error) {
	err = db.QueryRow(`SELECT COUNT(*) FROM aliases WHERE category = ?`, name).Scan(&aliases)
	if err != nil {
		return
	}
	err = db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE category = ?`, name).Scan(&transactions)
	return
}

// NextCategorySortOrder returns max(sort_order) + 1 for a sensible default.
func NextCategorySortOrder(db *sql.DB) int {
	var max int
	db.QueryRow(`SELECT COALESCE(MAX(sort_order), 0) FROM categories`).Scan(&max)
	return max + 1
}
