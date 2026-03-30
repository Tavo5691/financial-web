package queries

import (
	"database/sql"
	"fmt"
	"strings"
)

// ListCategoryNames returns all category names ordered by sort_order.
func ListCategoryNames(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM categories ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("list category names: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("scan category name: %w", err)
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// TrendPoint holds the total spending for one (period, category) pair.
type TrendPoint struct {
	Period   string
	Category string
	Total    float64
}

// GetCategoryTrend returns per-(period, category) spending totals for the given
// category and card filters. Only non-hidden debit transactions are included.
// Results are sorted by period ASC so the caller can build a time-series directly.
func GetCategoryTrend(db *sql.DB, categories []string, cards []string) ([]TrendPoint, error) {
	q := `SELECT statement_period, category, ROUND(SUM(amount_ars), 2)
	FROM transactions
	WHERE is_hidden = 0 AND is_debit = 1`

	var args []any

	if len(categories) > 0 {
		placeholders := make([]string, len(categories))
		for i, c := range categories {
			placeholders[i] = "?"
			args = append(args, c)
		}
		q += " AND category IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(cards) > 0 {
		cardConds := make([]string, 0, len(cards))
		for _, combo := range cards {
			if idx := strings.Index(combo, ":"); idx != -1 {
				cardConds = append(cardConds, "(card_type = ? AND card_bank = ?)")
				args = append(args, combo[:idx], combo[idx+1:])
			}
		}
		if len(cardConds) > 0 {
			q += " AND (" + strings.Join(cardConds, " OR ") + ")"
		}
	}

	q += " GROUP BY statement_period, category ORDER BY statement_period ASC"

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("get category trend: %w", err)
	}
	defer rows.Close()

	var result []TrendPoint
	for rows.Next() {
		var p TrendPoint
		if err := rows.Scan(&p.Period, &p.Category, &p.Total); err != nil {
			return nil, fmt.Errorf("scan trend point: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

// GetTopCategories returns the names of the top n categories by all-time spending.
func GetTopCategories(db *sql.DB, n int) ([]string, error) {
	rows, err := db.Query(`
		SELECT category
		FROM transactions
		WHERE is_hidden = 0 AND is_debit = 1
		GROUP BY category
		ORDER BY SUM(amount_ars) DESC
		LIMIT ?`, n)
	if err != nil {
		return nil, fmt.Errorf("get top categories: %w", err)
	}
	defer rows.Close()

	var cats []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("scan top category: %w", err)
		}
		cats = append(cats, c)
	}
	return cats, rows.Err()
}
