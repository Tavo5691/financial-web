package web

import (
	"financial-web/internal/db/queries"
	"financial-web/internal/domain"
	"net/http"
	"strconv"
)

// StatementReviewData is the view model for the statement review page.
type StatementReviewData struct {
	Statement    *domain.Statement
	Transactions []*domain.Transaction
}

func (s *Server) handleStatementReview(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	stmt, err := queries.GetStatementByID(s.DB, id)
	if err != nil {
		http.Error(w, "statement not found", http.StatusNotFound)
		return
	}
	txns, err := queries.ListByStatement(s.DB, id)
	if err != nil {
		s.Logger.Error("list by statement", "id", id, "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	s.render(w, "statement_review.html", StatementReviewData{
		Statement:    stmt,
		Transactions: txns,
	})
}
