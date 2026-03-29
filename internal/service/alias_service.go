package service

import (
	"database/sql"
	"financial-web/internal/domain"
	"financial-web/internal/db/queries"
	"fmt"
)

// AliasService handles alias CRUD and retroactive propagation.
type AliasService struct {
	DB *sql.DB
}

// Create inserts a new alias and applies it retroactively.
// Returns the new alias ID and the number of transactions updated.
func (s *AliasService) Create(pattern, merchant, category, expenseType, notes string) (*domain.Alias, int64, error) {
	a := &domain.Alias{
		Pattern:     pattern,
		Merchant:    merchant,
		Category:    category,
		ExpenseType: expenseType,
		Notes:       notes,
	}
	id, err := queries.InsertAlias(s.DB, a)
	if err != nil {
		return nil, 0, fmt.Errorf("create alias: %w", err)
	}
	a.ID = id

	affected, err := queries.ApplyAliasToTransactions(s.DB, id, pattern, merchant, category, expenseType)
	if err != nil {
		return a, 0, fmt.Errorf("apply alias: %w", err)
	}
	return a, affected, nil
}

// Update saves changes to an existing alias and re-applies it retroactively.
func (s *AliasService) Update(id int64, pattern, merchant, category, expenseType, notes string) (*domain.Alias, int64, error) {
	a := &domain.Alias{
		ID:          id,
		Pattern:     pattern,
		Merchant:    merchant,
		Category:    category,
		ExpenseType: expenseType,
		Notes:       notes,
	}
	if err := queries.UpdateAlias(s.DB, a); err != nil {
		return nil, 0, fmt.Errorf("update alias: %w", err)
	}

	affected, err := queries.ApplyAliasToTransactions(s.DB, id, pattern, merchant, category, expenseType)
	if err != nil {
		return a, 0, fmt.Errorf("apply alias: %w", err)
	}
	return a, affected, nil
}

// Delete removes an alias. Transactions that matched it keep their current merchant/category.
func (s *AliasService) Delete(id int64) error {
	return queries.DeleteAlias(s.DB, id)
}
