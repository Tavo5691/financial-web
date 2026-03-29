package queries

import (
	"database/sql"
	"fmt"
)

// InstallmentPlan holds the current state of an active installment plan.
// Fields marked "computed" are populated by the caller after the query.
type InstallmentPlan struct {
	Description   string
	Merchant      string
	CardType      string  // VISA | MASTERCARD
	CardBank      string  // GALICIA | BBVA | UALA
	Total         int     // installment_total
	LastCurrent   int     // most recent installment_current seen in the DB
	LastPeriod    string  // YYYY-MM of the statement where LastCurrent was seen
	MonthlyAmount float64 // amount_ars for that row — equals each future payment

	// Computed by handler:
	Remaining    int
	FinalPeriod  string
	TotalPending float64
}

// GetActiveInstallmentPlans returns all installment plans that still have
// future payments due (installment_current < installment_total), using the
// most recent known statement for each plan.
func GetActiveInstallmentPlans(db *sql.DB) ([]InstallmentPlan, error) {
	q := `
SELECT t.description_original, t.merchant,
       t.card_type, t.card_bank,
       t.installment_total, t.installment_current,
       t.statement_period, t.amount_ars
FROM transactions t
INNER JOIN (
    SELECT description_original, card_type, card_bank, installment_total,
           MAX(statement_period) AS max_period
    FROM transactions
    WHERE is_installment = 1 AND is_hidden = 0
    GROUP BY description_original, card_type, card_bank, installment_total
) latest
  ON  t.description_original = latest.description_original
  AND t.card_type             = latest.card_type
  AND t.card_bank             = latest.card_bank
  AND t.installment_total     = latest.installment_total
  AND t.statement_period      = latest.max_period
WHERE t.is_installment = 1
  AND t.is_hidden = 0
  AND t.installment_current < t.installment_total
ORDER BY t.card_type, t.card_bank, t.description_original`

	rows, err := db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("get active installment plans: %w", err)
	}
	defer rows.Close()

	var plans []InstallmentPlan
	for rows.Next() {
		var p InstallmentPlan
		if err := rows.Scan(
			&p.Description, &p.Merchant,
			&p.CardType, &p.CardBank,
			&p.Total, &p.LastCurrent,
			&p.LastPeriod, &p.MonthlyAmount,
		); err != nil {
			return nil, fmt.Errorf("scan installment plan: %w", err)
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}
