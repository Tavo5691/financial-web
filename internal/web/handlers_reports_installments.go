package web

import (
	"financial-web/internal/db/queries"
	"net/http"
	"sort"
	"time"
)

// MonthForecast holds the total installment burden for one future month.
type MonthForecast struct {
	Period    string
	Total     float64
	PlanNames []string
}

// InstallmentsReportData is the view model for the installments report page.
type InstallmentsReportData struct {
	Plans           []queries.InstallmentPlan
	Calendar        []MonthForecast
	TotalMonthlyNow float64 // sum of current monthly amounts across all active plans
	TotalPending    float64 // total ARS still owed across all plans
	LastPeriod      string  // latest month in the calendar
}

func (s *Server) handleReportsInstallments(w http.ResponseWriter, r *http.Request) {
	plans, err := queries.GetActiveInstallmentPlans(s.DB)
	if err != nil {
		s.Logger.Error("get active installment plans", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}

	calMap := map[string]*MonthForecast{}

	for i := range plans {
		p := &plans[i]
		p.Remaining = p.Total - p.LastCurrent
		p.FinalPeriod = addMonths(p.LastPeriod, p.Remaining)
		p.TotalPending = float64(p.Remaining) * p.MonthlyAmount

		name := p.Merchant
		if name == "" {
			name = p.Description
		}

		for j := 1; j <= p.Remaining; j++ {
			month := addMonths(p.LastPeriod, j)
			if calMap[month] == nil {
				calMap[month] = &MonthForecast{Period: month}
			}
			calMap[month].Total += p.MonthlyAmount
			calMap[month].PlanNames = append(calMap[month].PlanNames, name)
		}
	}

	// Sort calendar by period ascending.
	calendar := make([]MonthForecast, 0, len(calMap))
	for _, fc := range calMap {
		calendar = append(calendar, *fc)
	}
	sort.Slice(calendar, func(i, j int) bool {
		return calendar[i].Period < calendar[j].Period
	})

	var totalMonthlyNow, totalPending float64
	for _, p := range plans {
		totalMonthlyNow += p.MonthlyAmount
		totalPending += p.TotalPending
	}

	lastPeriod := ""
	if len(calendar) > 0 {
		lastPeriod = calendar[len(calendar)-1].Period
	}

	s.render(w, "reports_installments.html", InstallmentsReportData{
		Plans:           plans,
		Calendar:        calendar,
		TotalMonthlyNow: totalMonthlyNow,
		TotalPending:    totalPending,
		LastPeriod:      lastPeriod,
	})
}

// addMonths advances a YYYY-MM period string by n months.
func addMonths(period string, n int) string {
	t, err := time.Parse("2006-01", period)
	if err != nil {
		return period
	}
	return t.AddDate(0, n, 0).Format("2006-01")
}
