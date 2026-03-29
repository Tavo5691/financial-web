package web

import (
	"financial-web/internal/db/queries"
	"financial-web/internal/domain"
	"fmt"
	"net/http"
	"strconv"
)

// AliasesData is the view model for the aliases page.
type AliasesData struct {
	Aliases []*domain.Alias
}

// AliasFormData is the view model for the alias create/edit form.
type AliasFormData struct {
	Alias      *domain.Alias
	Categories []*domain.Category
}

func (s *Server) handleAliases(w http.ResponseWriter, r *http.Request) {
	aliases, err := queries.ListAliases(s.DB)
	if err != nil {
		s.Logger.Error("list aliases", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	s.render(w, "aliases.html", AliasesData{Aliases: aliases})
}

func (s *Server) handleAliasNew(w http.ResponseWriter, r *http.Request) {
	cats, _ := queries.ListCategories(s.DB)
	s.render(w, "alias_edit_form.html", AliasFormData{
		Alias:      &domain.Alias{},
		Categories: cats,
	})
}

func (s *Server) handleAliasCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	a, affected, err := s.AliasSvc.Create(
		r.FormValue("pattern"),
		r.FormValue("merchant"),
		r.FormValue("category"),
		r.FormValue("expense_type"),
		r.FormValue("notes"),
	)
	if err != nil {
		s.Logger.Error("create alias", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Trigger", fmt.Sprintf(
		`{"showToast":{"msg":"Alias creado. %d transacciones actualizadas.","type":"success"}}`, affected))
	s.render(w, "alias_row.html", a)
}

func (s *Server) handleAliasEdit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	a, err := queries.GetAliasByID(s.DB, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	cats, _ := queries.ListCategories(s.DB)
	s.render(w, "alias_edit_form.html", AliasFormData{Alias: a, Categories: cats})
}

func (s *Server) handleAliasView(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	a, err := queries.GetAliasByID(s.DB, id)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.render(w, "alias_row.html", a)
}

func (s *Server) handleAliasUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	a, affected, err := s.AliasSvc.Update(id,
		r.FormValue("pattern"),
		r.FormValue("merchant"),
		r.FormValue("category"),
		r.FormValue("expense_type"),
		r.FormValue("notes"),
	)
	if err != nil {
		s.Logger.Error("update alias", "id", id, "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Trigger", fmt.Sprintf(
		`{"showToast":{"msg":"Alias actualizado. %d transacciones actualizadas.","type":"success"}}`, affected))
	s.render(w, "alias_row.html", a)
}

func (s *Server) handleAliasDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.AliasSvc.Delete(id); err != nil {
		s.Logger.Error("delete alias", "id", id, "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
