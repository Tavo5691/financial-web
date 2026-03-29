package web

import (
	"financial-web/internal/db/queries"
	"net/http"
	"strings"
)

// MonthlyReportFilter holds the active filter state for the monthly report.
type MonthlyReportFilter struct {
	Period      string
	CardCombo   string // raw "VISA:GALICIA" for template selected-state
	Card        string
	CardBank    string
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
	cardParam := r.URL.Query().Get("card")
	filter := MonthlyReportFilter{
		Period:      r.URL.Query().Get("period"),
		CardCombo:   cardParam,
		ExpenseType: r.URL.Query().Get("expense_type"),
	}
	if idx := strings.Index(cardParam, ":"); idx != -1 {
		filter.Card = cardParam[:idx]
		filter.CardBank = cardParam[idx+1:]
	}

	periods := listPeriods(s.DB)

	// Default to the most recent period if none is selected.
	if filter.Period == "" && len(periods) > 0 {
		filter.Period = periods[0]
	}

	totals, err := queries.GetMonthlyCategoryTotals(s.DB, filter.Period, filter.Card, filter.CardBank, filter.ExpenseType)
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
	cardParam := r.URL.Query().Get("card")
	txFilter := queries.TransactionFilter{
		Period:      r.URL.Query().Get("period"),
		Category:    r.URL.Query().Get("category"),
		ExpenseType: r.URL.Query().Get("expense_type"),
	}
	if idx := strings.Index(cardParam, ":"); idx != -1 {
		txFilter.Card = cardParam[:idx]
		txFilter.CardBank = cardParam[idx+1:]
	}

	txns, err := queries.ListTransactions(s.DB, txFilter)
	if err != nil {
		s.Logger.Error("list transactions for detail", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}

	s.render(w, "monthly_txn_detail.html", txns)
}
