package web

import (
	"database/sql"
	"embed"
	"financial-web/internal/service"
	"financial-web/internal/watcher"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

//go:embed templates
var templatesFS embed.FS

var funcMap = template.FuncMap{
	"formatDate":    func(t time.Time) string { return t.Format("02/01/2006") },
	"formatMoney":   formatMoney,
	"truncate":      truncate,
	"statusLabel":   statusLabel,
	"expenseLabel":  expenseLabel,
	"urlPathEscape": url.PathEscape,
	"deref": func(f *float64) float64 {
		if f == nil {
			return 0
		}
		return *f
	},
	"dict": func(pairs ...any) map[string]any {
		m := map[string]any{}
		for i := 0; i+1 < len(pairs); i += 2 {
			m[pairs[i].(string)] = pairs[i+1]
		}
		return m
	},
	"contains": func(slice []string, val string) bool {
		for _, s := range slice {
			if s == val {
				return true
			}
		}
		return false
	},
	"cardsQuery": func(cards []string) string {
		parts := make([]string, 0, len(cards))
		for _, c := range cards {
			parts = append(parts, "card="+url.QueryEscape(c))
		}
		return strings.Join(parts, "&")
	},
	"join": strings.Join,
}

// Server holds app-level dependencies shared across handlers.
type Server struct {
	DB       *sql.DB
	Logger   *slog.Logger
	Watcher  *watcher.Watcher
	StmtSvc  *service.StatementService
	AliasSvc *service.AliasService
	// pageTmpls maps page template name → its own isolated template.Template
	// (layout + partials + the specific page). Avoids {{define "content"}} collisions.
	pageTmpls map[string]*template.Template
	// partialTmpl holds just the partials for rendering HTMX fragments.
	partialTmpl *template.Template
}

// New creates a Server, pre-builds all template sets, and wires up the watcher.
func New(db *sql.DB, logger *slog.Logger, inboxDir, processedDir string) (*Server, error) {
	pageTmpls, partialTmpl, err := buildTemplateSets()
	if err != nil {
		return nil, err
	}

	w := &watcher.Watcher{
		InboxDir: inboxDir,
		DB:       db,
		Logger:   logger,
	}

	s := &Server{
		DB:          db,
		Logger:      logger,
		Watcher:     w,
		StmtSvc:     &service.StatementService{DB: db, ProcessedDir: processedDir},
		AliasSvc:    &service.AliasService{DB: db},
		pageTmpls:   pageTmpls,
		partialTmpl: partialTmpl,
	}
	return s, nil
}

// Handler returns the root http.Handler with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Static files served from ./static on disk.
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))

	// Redirect root
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/inbox", http.StatusFound)
	})

	// Inbox
	mux.HandleFunc("GET /inbox", s.handleInbox)
	mux.HandleFunc("GET /inbox/scan", s.handleInboxScan)
	mux.HandleFunc("GET /inbox/{id}/process-form", s.handleProcessForm)
	mux.HandleFunc("POST /inbox/process", s.handleProcess)

	// Statements
	mux.HandleFunc("GET /statements/{id}/review", s.handleStatementReview)

	// Transactions
	mux.HandleFunc("GET /transactions", s.handleTransactions)
	mux.HandleFunc("GET /transactions/{id}/edit", s.handleTransactionEdit)
	mux.HandleFunc("GET /transactions/{id}/view", s.handleTransactionView)
	mux.HandleFunc("PATCH /transactions/{id}", s.handleTransactionUpdate)
	mux.HandleFunc("POST /transactions/{id}/hide", s.handleTransactionHide)

	// Aliases
	mux.HandleFunc("GET /aliases", s.handleAliases)
	mux.HandleFunc("GET /aliases/new", s.handleAliasNew)
	mux.HandleFunc("POST /aliases", s.handleAliasCreate)
	mux.HandleFunc("GET /aliases/{id}/edit", s.handleAliasEdit)
	mux.HandleFunc("GET /aliases/{id}/view", s.handleAliasView)
	mux.HandleFunc("PATCH /aliases/{id}", s.handleAliasUpdate)
	mux.HandleFunc("DELETE /aliases/{id}", s.handleAliasDelete)

	// Categories
	mux.HandleFunc("GET /categories", s.handleCategories)
	mux.HandleFunc("GET /categories/new", s.handleCategoryNew)
	mux.HandleFunc("POST /categories", s.handleCategoryCreate)
	mux.HandleFunc("GET /categories/{name}/edit", s.handleCategoryEdit)
	mux.HandleFunc("GET /categories/{name}/view", s.handleCategoryView)
	mux.HandleFunc("PATCH /categories/{name}", s.handleCategoryUpdate)
	mux.HandleFunc("DELETE /categories/{name}", s.handleCategoryDelete)

	// Reports
	mux.HandleFunc("GET /reports/monthly",        s.handleReportsMonthly)
	mux.HandleFunc("GET /reports/monthly/detail", s.handleReportsMonthlyDetail)
	mux.HandleFunc("GET /reports/installments",   s.handleReportsInstallments)
	mux.HandleFunc("GET /reports/trend",          s.handleReportsTrend)

	// Parsers
	mux.HandleFunc("GET /parsers", s.handleParsers)

	return mux
}

// isHTMX returns true if the request was issued by HTMX.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// render executes the named template.
// For page templates (*.html at top level) it uses the isolated per-page set and
// executes "layout.html" as the entry point.
// For partials (inbox_list, transaction_row, etc.) it uses the shared partial set.
func (s *Server) render(w http.ResponseWriter, name string, data any) {
	// Page templates: look up the isolated set and run layout.html
	if t, ok := s.pageTmpls[name]; ok {
		if err := t.ExecuteTemplate(w, "layout.html", data); err != nil {
			s.Logger.Error("render page", "name", name, "error", err)
		}
		return
	}
	// Partials: run the named define directly
	if err := s.partialTmpl.ExecuteTemplate(w, name, data); err != nil {
		s.Logger.Error("render partial", "name", name, "error", err)
	}
}

// buildTemplateSets constructs:
//   - pageTmpls: one *template.Template per page (layout + partials + the page itself)
//   - partialTmpl: all partials in a single set (for HTMX fragment rendering)
func buildTemplateSets() (map[string]*template.Template, *template.Template, error) {
	// Collect all template sources from the embedded FS.
	type tmplSrc struct {
		name    string
		content string
		isPage  bool // true = top-level page; false = layout/partial
	}

	var layout string
	pages := map[string]string{}   // basename → source
	partials := map[string]string{} // define-name → source

	err := fs.WalkDir(templatesFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		raw, err := templatesFS.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(raw)
		base := filepath.Base(path)

		switch {
		case base == "layout.html":
			layout = src
		case strings.Contains(path, "partials/"):
			partials[base] = src
		default:
			// Top-level page template
			pages[base] = src
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	// Build the partial set (used for HTMX fragments).
	partialTmpl := template.New("").Funcs(funcMap)
	for name, src := range partials {
		// Strip the {{define "..."}}/{{end}} wrapper to get just the inner name.
		// Actually we want to keep it so we can call ExecuteTemplate by define name.
		if _, err := partialTmpl.New(name).Parse(src); err != nil {
			return nil, nil, err
		}
	}

	// Build one isolated template set per page.
	pageTmpls := map[string]*template.Template{}
	for pageName, pageSrc := range pages {
		t := template.New("").Funcs(funcMap)
		// Add layout
		if _, err := t.New("layout.html").Parse(layout); err != nil {
			return nil, nil, err
		}
		// Add all partials
		for pName, pSrc := range partials {
			if _, err := t.New(pName).Parse(pSrc); err != nil {
				return nil, nil, err
			}
		}
		// Add the page itself (overrides "content" and "title" blocks for THIS page only)
		if _, err := t.New(pageName).Parse(pageSrc); err != nil {
			return nil, nil, err
		}
		pageTmpls[pageName] = t
	}

	return pageTmpls, partialTmpl, nil
}

// --- Formatting helpers ---

func formatMoney(f float64) string { return formatFloat(f) }

func formatFloat(f float64) string {
	neg := f < 0
	if neg {
		f = -f
	}
	intPart := int64(f)
	frac := int64((f-float64(intPart))*100 + 0.5)

	intStr := thousandsSep(intPart)
	result := intStr + "," + padLeft2(int(frac))
	if neg {
		result = "-" + result
	}
	return result
}

func thousandsSep(n int64) string {
	s := intToStr(n)
	result := []byte{}
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, '.')
		}
		result = append(result, byte(c))
	}
	return string(result)
}

func intToStr(n int64) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func padLeft2(n int) string {
	if n < 10 {
		return "0" + intToStr(int64(n))
	}
	return intToStr(int64(n))
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

func statusLabel(s string) string {
	switch s {
	case "pending":
		return "Pendiente"
	case "processing":
		return "Procesando"
	case "reviewed":
		return "Revisado"
	case "error":
		return "Error"
	}
	return s
}

func expenseLabel(s string) string {
	switch s {
	case "fixed":
		return "Fijo"
	case "variable":
		return "Variable"
	case "installment":
		return "Cuotas"
	}
	return s
}
