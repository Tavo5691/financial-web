package web

import (
	"database/sql"
	"financial-web/internal/db/queries"
	"financial-web/internal/domain"
	"net/http"
	"sort"
)

// TransactionsData is the view model for the transactions page.
type TransactionsData struct {
	Transactions []*domain.Transaction
	Filter       queries.TransactionFilter
	Categories   []*domain.Category
	Periods      []string
}

// EditFormData is the view model for the inline transaction edit form.
type EditFormData struct {
	Transaction *domain.Transaction
	Categories  []*domain.Category
}

func (s *Server) handleTransactions(w http.ResponseWriter, r *http.Request) {
	filter := queries.TransactionFilter{
		Period:   r.URL.Query().Get("period"),
		Category: r.URL.Query().Get("category"),
		Search:   r.URL.Query().Get("q"),
		Cards:    r.URL.Query()["card"],
	}

	txns, err := queries.ListTransactions(s.DB, filter)
	if err != nil {
		s.Logger.Error("list transactions", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}

	cats, _ := queries.ListCategories(s.DB)
	sort.Slice(cats, func(i, j int) bool { return cats[i].Name < cats[j].Name })
	periods := listPeriods(s.DB)

	// HTMX partial: return only the rows.
	if isHTMX(r) {
		for _, t := range txns {
			s.render(w, "transaction_row.html", t)
		}
		return
	}

	s.render(w, "transactions.html", TransactionsData{
		Transactions: txns,
		Filter:       filter,
		Categories:   cats,
		Periods:      periods,
	})
}

func (s *Server) handleTransactionEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	txn, err := queries.GetTransactionByID(s.DB, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	cats, _ := queries.ListCategories(s.DB)
	s.render(w, "transaction_edit_form.html", EditFormData{Transaction: txn, Categories: cats})
}

func (s *Server) handleTransactionView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	txn, err := queries.GetTransactionByID(s.DB, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.render(w, "transaction_row.html", txn)
}

func (s *Server) handleTransactionUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := queries.UpdateTransaction(s.DB, id,
		r.FormValue("merchant"),
		r.FormValue("category"),
		r.FormValue("expense_type"),
		r.FormValue("notes"),
	); err != nil {
		s.Logger.Error("update transaction", "id", id, "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	txn, err := queries.GetTransactionByID(s.DB, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.render(w, "transaction_row.html", txn)
}

func (s *Server) handleTransactionHide(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := queries.ToggleHidden(s.DB, id); err != nil {
		s.Logger.Error("toggle hidden", "id", id, "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	txn, err := queries.GetTransactionByID(s.DB, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.render(w, "transaction_row.html", txn)
}

func listPeriods(db *sql.DB) []string {
	rows, err := db.Query(`SELECT DISTINCT statement_period FROM transactions ORDER BY statement_period DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var periods []string
	for rows.Next() {
		var p string
		rows.Scan(&p)
		periods = append(periods, p)
	}
	return periods
}
