package web

import (
	"encoding/json"
	"financial-web/internal/db/queries"
	"html/template"
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

// trendColors is a fixed 12-color palette for Chart.js datasets.
var trendColors = []string{
	"#6366f1", "#22c55e", "#f59e0b", "#ef4444", "#3b82f6", "#ec4899",
	"#14b8a6", "#f97316", "#8b5cf6", "#84cc16", "#06b6d4", "#fb7185",
}

// TrendFilter holds the active filter state for the trend report.
type TrendFilter struct {
	Categories []string // selected category names
	Cards      []string // "CARDTYPE:BANK" combos, empty = all
}

// TrendData is the view model for the trend report page.
type TrendData struct {
	Filter        TrendFilter
	AllCategories []string
	Labels        template.JS // JSON array of period strings
	Datasets      template.JS // JSON array of Chart.js dataset objects
}

func (s *Server) handleReportsTrend(w http.ResponseWriter, r *http.Request) {
	filter := TrendFilter{
		Categories: r.URL.Query()["category"],
		Cards:      r.URL.Query()["card"],
	}

	// Default to top 8 categories when none are selected.
	if len(filter.Categories) == 0 {
		top, err := queries.GetTopCategories(s.DB, 8)
		if err != nil {
			s.Logger.Error("get top categories", "error", err)
			http.Error(w, "error", http.StatusInternalServerError)
			return
		}
		filter.Categories = top
	}

	allCats, err := queries.ListCategoryNames(s.DB)
	if err != nil {
		s.Logger.Error("list category names", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}

	points, err := queries.GetCategoryTrend(s.DB, filter.Categories, filter.Cards)
	if err != nil {
		s.Logger.Error("get category trend", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}

	labels, datasets := buildChartData(points, filter.Categories)

	labelsJSON, _ := json.Marshal(labels)
	datasetsJSON, _ := json.Marshal(datasets)

	data := TrendData{
		Filter:        filter,
		AllCategories: allCats,
		Labels:        template.JS(labelsJSON),
		Datasets:      template.JS(datasetsJSON),
	}

	s.render(w, "reports_trend.html", data)
}

// chartDataset is a Chart.js line dataset object.
type chartDataset struct {
	Label       string    `json:"label"`
	Data        []float64 `json:"data"`
	BorderColor string    `json:"borderColor"`
	BackgroundColor string `json:"backgroundColor"`
	Tension     float64   `json:"tension"`
	PointRadius int       `json:"pointRadius"`
}

// buildChartData pivots flat TrendPoints into Chart.js labels + datasets.
func buildChartData(points []queries.TrendPoint, categories []string) ([]string, []chartDataset) {
	// Build sorted period list (points are already ORDER BY period ASC, so we
	// can collect in order without a sort).
	var periods []string
	seen := map[string]bool{}
	for _, p := range points {
		if !seen[p.Period] {
			periods = append(periods, p.Period)
			seen[p.Period] = true
		}
	}

	// Build a lookup: category → period → total.
	lookup := map[string]map[string]float64{}
	for _, p := range points {
		if _, ok := lookup[p.Category]; !ok {
			lookup[p.Category] = map[string]float64{}
		}
		lookup[p.Category][p.Period] = p.Total
	}

	datasets := make([]chartDataset, 0, len(categories))
	for i, cat := range categories {
		color := trendColors[i%len(trendColors)]
		data := make([]float64, len(periods))
		for j, period := range periods {
			data[j] = lookup[cat][period] // zero if missing
		}
		datasets = append(datasets, chartDataset{
			Label:           cat,
			Data:            data,
			BorderColor:     color,
			BackgroundColor: color + "33", // ~20% opacity hex
			Tension:         0.3,
			PointRadius:     3,
		})
	}

	return periods, datasets
}
