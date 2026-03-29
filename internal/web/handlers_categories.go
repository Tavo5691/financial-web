package web

import (
	"financial-web/internal/db/queries"
	"financial-web/internal/domain"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// CategoriesData is the view model for the categories list page.
type CategoriesData struct {
	Categories []*domain.Category
}

// CategoryFormData is the view model for the category create/edit form.
type CategoryFormData struct {
	Category *domain.Category
	IsNew    bool
}

func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := queries.ListCategories(s.DB)
	if err != nil {
		s.Logger.Error("list categories", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	s.render(w, "categories.html", CategoriesData{Categories: cats})
}

func (s *Server) handleCategoryNew(w http.ResponseWriter, r *http.Request) {
	s.render(w, "category_edit_form.html", CategoryFormData{
		Category: &domain.Category{SortOrder: queries.NextCategorySortOrder(s.DB)},
		IsNew:    true,
	})
}

func (s *Server) handleCategoryCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	c := &domain.Category{
		Name:               r.FormValue("name"),
		DefaultExpenseType: r.FormValue("default_expense_type"),
		Keywords:           r.FormValue("keywords"),
		SortOrder:          sortOrder,
	}
	if err := queries.InsertCategory(s.DB, c); err != nil {
		s.Logger.Error("create category", "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Trigger", `{"showToast":{"msg":"Categoría creada.","type":"success"}}`)
	s.render(w, "category_row.html", c)
}

func (s *Server) handleCategoryEdit(w http.ResponseWriter, r *http.Request) {
	name, err := url.PathUnescape(r.PathValue("name"))
	if err != nil {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	c, err := queries.GetCategoryByName(s.DB, name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.render(w, "category_edit_form.html", CategoryFormData{Category: c, IsNew: false})
}

func (s *Server) handleCategoryView(w http.ResponseWriter, r *http.Request) {
	name, err := url.PathUnescape(r.PathValue("name"))
	if err != nil {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	c, err := queries.GetCategoryByName(s.DB, name)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	s.render(w, "category_row.html", c)
}

func (s *Server) handleCategoryUpdate(w http.ResponseWriter, r *http.Request) {
	oldName, err := url.PathUnescape(r.PathValue("name"))
	if err != nil {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	sortOrder, _ := strconv.Atoi(r.FormValue("sort_order"))
	c := &domain.Category{
		Name:               r.FormValue("name"),
		DefaultExpenseType: r.FormValue("default_expense_type"),
		Keywords:           r.FormValue("keywords"),
		SortOrder:          sortOrder,
	}
	if err := queries.UpdateCategory(s.DB, oldName, c); err != nil {
		s.Logger.Error("update category", "name", oldName, "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	msg := "Categoría actualizada."
	if c.Name != oldName {
		msg = fmt.Sprintf("Categoría renombrada de «%s» a «%s».", oldName, c.Name)
	}
	w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"msg":%q,"type":"success"}}`, msg))
	s.render(w, "category_row.html", c)
}

func (s *Server) handleCategoryDelete(w http.ResponseWriter, r *http.Request) {
	name, err := url.PathUnescape(r.PathValue("name"))
	if err != nil {
		http.Error(w, "invalid name", http.StatusBadRequest)
		return
	}
	aliasCount, txCount, err := queries.CountCategoryUsage(s.DB, name)
	if err != nil {
		s.Logger.Error("count category usage", "name", name, "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	if aliasCount > 0 || txCount > 0 {
		msg := fmt.Sprintf("No se puede eliminar: %d alias y %d transacciones usan esta categoría.", aliasCount, txCount)
		w.Header().Set("HX-Trigger", fmt.Sprintf(`{"showToast":{"msg":%q,"type":"error"}}`, msg))
		w.WriteHeader(http.StatusConflict)
		return
	}
	if err := queries.DeleteCategory(s.DB, name); err != nil {
		s.Logger.Error("delete category", "name", name, "error", err)
		http.Error(w, "error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
