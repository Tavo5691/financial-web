package categorizer

import (
	"database/sql"
	"financial-web/internal/domain"
	"strings"
)

// Result holds the categorization outcome for a single transaction description.
type Result struct {
	Merchant    string
	Category    string
	ExpenseType string
	AliasID     *int64
}

// Categorize resolves the merchant, category, and expense type for a raw description.
// Priority:
//  1. Alias match (description_pattern LIKE match)
//  2. Category keyword match
//  3. Fallback: "Otros" / "variable"
func Categorize(db *sql.DB, descriptionOriginal string, aliases []*domain.Alias, categories []*domain.Category) Result {
	upper := strings.ToUpper(descriptionOriginal)

	// 1. Alias lookup
	for _, a := range aliases {
		if strings.Contains(upper, a.Pattern) { // pattern is already stored uppercase
			id := a.ID
			return Result{
				Merchant:    a.Merchant,
				Category:    a.Category,
				ExpenseType: a.ExpenseType,
				AliasID:     &id,
			}
		}
	}

	// 2. Category keyword match
	for _, c := range categories {
		if c.Keywords == "" {
			continue
		}
		for _, kw := range strings.Split(c.Keywords, ",") {
			kw = strings.TrimSpace(strings.ToUpper(kw))
			if kw != "" && strings.Contains(upper, kw) {
				return Result{
					Merchant:    titleCase(descriptionOriginal),
					Category:    c.Name,
					ExpenseType: c.DefaultExpenseType,
				}
			}
		}
	}

	// 3. Fallback
	return Result{
		Merchant:    titleCase(descriptionOriginal),
		Category:    "Otros",
		ExpenseType: "variable",
	}
}

// titleCase converts "SOME MERCHANT NAME" to "Some Merchant Name".
func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, " ")
}
