package pdf

import (
	"bytes"
	"fmt"
	"os"

	"github.com/ledongthuc/pdf"
)

// ExtractText reads a PDF file and returns all its text content as a single string.
// Pages are separated by newlines. This works for digitally-generated PDFs (not scanned).
func ExtractText(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("open pdf %q: %w", path, err)
	}
	defer f.Close()

	var buf bytes.Buffer
	for i := 1; i <= r.NumPage(); i++ {
		page := r.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			return "", fmt.Errorf("extract page %d: %w", i, err)
		}
		buf.WriteString(text)
		buf.WriteByte('\n')
	}
	return buf.String(), nil
}

// MustExtractText is like ExtractText but exits on error — used in CLI tools.
func MustExtractText(path string) string {
	text, err := ExtractText(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return text
}
