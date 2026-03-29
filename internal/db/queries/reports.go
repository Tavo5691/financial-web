package queries

import (
	"database/sql"
	"fmt"
)

// CategoryTotal holds the aggregated spending for one category.
type CategoryTotal struct {
	Category    string
	Count       int
	Total       float64
	Fixed       float64
	Variable    float64
	Installment float64
	Pct         float64 // percentage of grand total; computed by caller
}

// GetMonthlyCategoryTotals returns per-category spending totals for the given
// filters. All non-hidden transactions are included; amounts are in ARS.
func GetMonthlyCategoryTotals(db *sql.DB, period, card, cardBank, expenseType string) ([]CategoryTotal, error) {
	q := `SELECT category,
		COUNT(*),
		ROUND(SUM(amount_ars), 2),
		ROUND(SUM(CASE WHEN expense_type = 'fixed'       THEN amount_ars ELSE 0 END), 2),
		ROUND(SUM(CASE WHEN expense_type = 'variable'    THEN amount_ars ELSE 0 END), 2),
		ROUND(SUM(CASE WHEN expense_type = 'installment' THEN amount_ars ELSE 0 END), 2)
	FROM transactions
	WHERE is_hidden = 0`

	var args []any
	if period != "" {
		q += " AND statement_period = ?"
		args = append(args, period)
	}
	if card != "" {
		q += " AND card_type = ?"
		args = append(args, card)
	}
	if cardBank != "" {
		q += " AND card_bank = ?"
		args = append(args, cardBank)
	}
	if expenseType != "" {
		q += " AND expense_type = ?"
		args = append(args, expenseType)
	}
	q += " GROUP BY category ORDER BY SUM(amount_ars) DESC"

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("get monthly category totals: %w", err)
	}
	defer rows.Close()

	var result []CategoryTotal
	for rows.Next() {
		var ct CategoryTotal
		if err := rows.Scan(&ct.Category, &ct.Count, &ct.Total, &ct.Fixed, &ct.Variable, &ct.Installment); err != nil {
			return nil, fmt.Errorf("scan category total: %w", err)
		}
		result = append(result, ct)
	}
	return result, rows.Err()
}
