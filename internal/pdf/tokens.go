package pdf

import (
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ExtractTokens extracts raw Tj text tokens from a PDF whose content is stored
// in XObject form streams (e.g. BBVA digital statements).
// Returns a flat slice of all non-empty tokens across all pages in order.
func ExtractTokens(path string) ([]string, error) {
	ctx, err := api.ReadContextFile(path)
	if err != nil {
		return nil, err
	}
	xref := ctx.XRefTable
	var all []string
	for pageNum := 1; pageNum <= ctx.PageCount; pageNum++ {
		d, _, _, err := ctx.PageDict(pageNum, false)
		if err != nil {
			continue
		}
		res := derefDict(xref, d["Resources"])
		if res == nil {
			continue
		}
		xobjs := derefDict(xref, res["XObject"])
		if xobjs == nil {
			continue
		}
		// Sort keys for deterministic order within a page.
		keys := make([]string, 0, len(xobjs))
		for k := range xobjs {
			keys = append(keys, string(k))
		}
		sort.Strings(keys)
		for _, k := range keys {
			ir, ok := xobjs[k].(types.IndirectRef)
			if !ok {
				continue
			}
			content := decodeXObj(xref, int(ir.ObjectNumber))
			all = append(all, parseTjTokens(content)...)
		}
	}
	return all, nil
}

func derefDict(xref *model.XRefTable, o types.Object) types.Dict {
	if d, ok := o.(types.Dict); ok {
		return d
	}
	if ir, ok := o.(types.IndirectRef); ok {
		obj, err := xref.Dereference(ir)
		if err != nil {
			return nil
		}
		if d, ok := obj.(types.Dict); ok {
			return d
		}
	}
	return nil
}

func decodeXObj(xref *model.XRefTable, objNum int) string {
	ir := types.IndirectRef{ObjectNumber: types.Integer(objNum)}
	sd, _, err := xref.DereferenceStreamDict(ir)
	if err != nil || sd == nil {
		return ""
	}
	_ = sd.Decode()
	return string(sd.Content)
}

func parseTjTokens(content string) []string {
	var tokens []string
	inBT := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "BT" {
			inBT = true
			continue
		}
		if line == "ET" {
			inBT = false
			continue
		}
		if !inBT || !strings.HasSuffix(line, "Tj") {
			continue
		}
		s := strings.TrimSpace(strings.TrimSuffix(line, "Tj"))
		if !strings.HasSuffix(s, ")") {
			continue
		}
		start := strings.Index(s, "(")
		if start < 0 {
			continue
		}
		raw := s[start+1 : len(s)-1]
		raw = strings.ReplaceAll(raw, `\(`, "(")
		raw = strings.ReplaceAll(raw, `\)`, ")")
		raw = strings.ReplaceAll(raw, `\\`, `\`)
		raw = strings.TrimSpace(raw)
		if raw != "" {
			tokens = append(tokens, raw)
		}
	}
	return tokens
}
