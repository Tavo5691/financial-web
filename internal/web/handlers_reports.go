package web

import (
	"financial-web/internal/db/queries"
	"net/http"
)

// MonthlyReportFilter holds the active filter state for the monthly report.
type MonthlyReportFilter struct {
	Period      string
	Cards       []string // "CARDTYPE:BANK" combos, empty = all
	ExpenseType string
}

// MonthlyReportData is the view model for the monthly report page.
type MonthlyReportData struct {
	Filter         MonthlyReportFilter
	Periods        []string
	CategoryTotals []queries.CategoryTotal
	GrandTotal     float64
	GrandCount     int
}

func (s *Server) handleReportsMonthly(w http.ResponseWriter, r *http.Request) {
	filter := MonthlyReportFilter{
		Period:      r.URL.Query().Get("period"),
		Cards:       r.URL.Query()["card"],
		ExpenseType: r.URL.Query().Get("expense_type"),
	}

	periods := listPeriods(s.DB)

	// Default to the most recent period if none is selected.
	if filter.Period == "" && len(periods) > 0 {
		filter.Period = periods[0]
	}

	totals, err := queries.GetMonthlyCategoryTotals(s.DB, filter.Period, filter.Cards, filter.ExpenseType)
	if err != nil {
		s.Logger.Error("get monthly category totals", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}

	var grandTotal float64
	var grandCount int
	for i := range totals {
		grandTotal += totals[i].Total
		grandCount += totals[i].Count
	}
	for i := range totals {
		if grandTotal > 0 {
			totals[i].Pct = totals[i].Total / grandTotal * 100
		}
	}

	data := MonthlyReportData{
		Filter:         filter,
		Periods:        periods,
		CategoryTotals: totals,
		GrandTotal:     grandTotal,
		GrandCount:     grandCount,
	}

	if isHTMX(r) {
		s.render(w, "monthly_report_body.html", data)
		return
	}
	s.render(w, "reports_monthly.html", data)
}

func (s *Server) handleReportsMonthlyDetail(w http.ResponseWriter, r *http.Request) {
	txFilter := queries.TransactionFilter{
		Period:      r.URL.Query().Get("period"),
		Cards:       r.URL.Query()["card"],
		Category:    r.URL.Query().Get("category"),
		ExpenseType: r.URL.Query().Get("expense_type"),
	}

	txns, err := queries.ListTransactions(s.DB, txFilter)
	if err != nil {
		s.Logger.Error("list transactions for detail", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}

	s.render(w, "monthly_txn_detail.html", txns)
}
