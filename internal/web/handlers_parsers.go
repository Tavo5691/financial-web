package web

import (
	"financial-web/internal/parser"
	"net/http"
)

// ParsersData is the view model for the parsers page.
type ParsersData struct {
	Parsers []parser.ParserInfo
}

func (s *Server) handleParsers(w http.ResponseWriter, r *http.Request) {
	all := parser.All()
	infos := make([]parser.ParserInfo, len(all))
	for i, p := range all {
		infos[i] = p.Info()
	}
	s.render(w, "parsers.html", ParsersData{Parsers: infos})
}
