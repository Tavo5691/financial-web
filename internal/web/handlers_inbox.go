package web

import (
	"financial-web/internal/db/queries"
	"financial-web/internal/domain"
	"financial-web/internal/parser"
	"net/http"
	"path/filepath"
	"strconv"
)

// StatementVM is the view model for a single statement row.
type StatementVM struct {
	*domain.Statement
	FileName string
}

func newStatementVM(s *domain.Statement) *StatementVM {
	return &StatementVM{
		Statement: s,
		FileName:  filepath.Base(s.SourceFile),
	}
}

// InboxData is the view model for the inbox page and its HTMX partial.
type InboxData struct {
	Statements []*StatementVM
}

func (s *Server) handleInbox(w http.ResponseWriter, r *http.Request) {
	stmts, err := queries.ListStatements(s.DB)
	if err != nil {
		s.Logger.Error("list statements", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	vms := make([]*StatementVM, len(stmts))
	for i, stmt := range stmts {
		vms[i] = newStatementVM(stmt)
	}
	s.render(w, "inbox.html", InboxData{Statements: vms})
}

func (s *Server) handleInboxScan(w http.ResponseWriter, r *http.Request) {
	s.Watcher.Scan()

	stmts, err := queries.ListStatements(s.DB)
	if err != nil {
		s.Logger.Error("list statements", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	vms := make([]*StatementVM, len(stmts))
	for i, stmt := range stmts {
		vms[i] = newStatementVM(stmt)
	}
	s.render(w, "inbox_list.html", InboxData{Statements: vms})
}

// ProcessFormData is the view model for the process modal.
type ProcessFormData struct {
	StatementID int64
	FileName    string
	Parsers     []parser.ParserInfo
}

func (s *Server) handleProcessForm(w http.ResponseWriter, r *http.Request) {
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
	parsers := parser.All()
	infos := make([]parser.ParserInfo, len(parsers))
	for i, p := range parsers {
		infos[i] = p.Info()
	}
	s.render(w, "process_form.html", ProcessFormData{
		StatementID: stmt.ID,
		FileName:    filepath.Base(stmt.SourceFile),
		Parsers:     infos,
	})
}

func (s *Server) handleProcess(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	stmtID, err := strconv.ParseInt(r.FormValue("statement_id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid statement_id", http.StatusBadRequest)
		return
	}
	parserID := r.FormValue("parser_id")
	if parserID == "" {
		http.Error(w, "parser_id required", http.StatusBadRequest)
		return
	}
	var exchangeRate float64
	if rateStr := r.FormValue("exchange_rate"); rateStr != "" {
		exchangeRate, _ = strconv.ParseFloat(rateStr, 64)
	}

	result, err := s.StmtSvc.Process(stmtID, parserID, exchangeRate)
	if err != nil {
		s.Logger.Error("process statement", "id", stmtID, "error", err)
		stmts, _ := queries.ListStatements(s.DB)
		vms := make([]*StatementVM, len(stmts))
		for i, st := range stmts {
			vms[i] = newStatementVM(st)
		}
		s.render(w, "inbox_list.html", InboxData{Statements: vms})
		return
	}

	http.Redirect(w, r, "/statements/"+strconv.FormatInt(result.StatementID, 10)+"/review", http.StatusSeeOther)
}
