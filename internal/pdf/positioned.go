package pdf

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
)

// TextItem is a positioned text token extracted from a PDF page.
type TextItem struct {
	Text string
	X    float64
	Y    float64
	Page int
}

// ExtractPositioned returns all text tokens with their coordinates, in reading
// order: page ascending, Y descending (top→bottom), X ascending (left→right).
func ExtractPositioned(path string) ([]TextItem, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open pdf %q: %w", path, err)
	}
	defer f.Close()

	var items []TextItem
	for pageNum := 1; pageNum <= r.NumPage(); pageNum++ {
		p := r.Page(pageNum)
		if p.V.IsNull() {
			continue
		}
		content := p.Content()
		for _, t := range content.Text {
			if t.S == "" {
				continue
			}
			items = append(items, TextItem{
				Text: t.S,
				X:    t.X,
				Y:    t.Y,
				Page: pageNum,
			})
		}
	}

	// Sort by page, then Y descending (top of page = high Y), then X ascending.
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Page != b.Page {
			return a.Page < b.Page
		}
		if math.Abs(a.Y-b.Y) > 1e-3 {
			return a.Y > b.Y
		}
		return a.X < b.X
	})

	return items, nil
}

// CollectColumnText assembles the text of all items whose X coordinate falls
// within [xMin, xMax], joining them without separators.
// Useful for extracting a single table column from a positioned row.
func CollectColumnText(row []TextItem, xMin, xMax float64) string {
	var b strings.Builder
	for _, item := range row {
		if item.X >= xMin && item.X <= xMax {
			b.WriteString(item.Text)
		}
	}
	return b.String()
}

// HasDigitInColumn reports whether any item in the row whose X falls within
// [xMin, xMax] contains at least one ASCII digit.
func HasDigitInColumn(row []TextItem, xMin, xMax float64) bool {
	for _, item := range row {
		if item.X < xMin || item.X > xMax {
			continue
		}
		for _, c := range item.Text {
			if c >= '0' && c <= '9' {
				return true
			}
		}
	}
	return false
}

// GroupByRow clusters TextItems into rows by Y coordinate within ±tolerance.
// Returns groups sorted top→bottom (Y descending across groups), each group
// sorted left→right (X ascending).
func GroupByRow(items []TextItem, tolerance float64) [][]TextItem {
	if len(items) == 0 {
		return nil
	}

	var rows [][]TextItem
	var current []TextItem
	currentY := items[0].Y

	for _, item := range items {
		if math.Abs(item.Y-currentY) <= tolerance {
			current = append(current, item)
		} else {
			rows = append(rows, current)
			current = []TextItem{item}
			currentY = item.Y
		}
	}
	if len(current) > 0 {
		rows = append(rows, current)
	}

	// Each group is already sorted by X (from ExtractPositioned).
	return rows
}
