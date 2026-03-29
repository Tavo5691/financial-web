package queries

import (
	"database/sql"
	"financial-web/internal/domain"
	"fmt"
	"time"
)

// StatementRow mirrors the DB columns for scanning.
type StatementRow struct {
	db *sql.DB
}

func NewStatementRow(db *sql.DB) *StatementRow { return &StatementRow{db: db} }

// InsertPending inserts a new statement with status=pending.
// Uses INSERT OR IGNORE so calling it twice on the same source_file is safe.
func InsertPending(db *sql.DB, sourceFile, fileHash string) (int64, error) {
	res, err := db.Exec(`
		INSERT OR IGNORE INTO statements (source_file, file_hash, card_type, card_bank, statement_period, status)
		VALUES (?, ?, '', '', '', 'pending')
	`, sourceFile, fileHash)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	// If LastInsertId is 0 the row already existed — look it up.
	if id == 0 {
		row := db.QueryRow(`SELECT id FROM statements WHERE source_file = ?`, sourceFile)
		if err := row.Scan(&id); err != nil {
			return 0, err
		}
	}
	return id, nil
}

// GetByID returns a statement by its primary key.
func GetStatementByID(db *sql.DB, id int64) (*domain.Statement, error) {
	row := db.QueryRow(`
		SELECT id, source_file, file_hash, card_type, card_bank, statement_period,
		       parser_id, status, processed_at, total_txns, total_amount_ars,
		       exchange_rate_usd, error_message, notes, created_at
		FROM statements WHERE id = ?`, id)
	return scanStatement(row)
}

// ListAll returns all statements ordered by created_at desc.
func ListStatements(db *sql.DB) ([]*domain.Statement, error) {
	rows, err := db.Query(`
		SELECT id, source_file, file_hash, card_type, card_bank, statement_period,
		       parser_id, status, processed_at, total_txns, total_amount_ars,
		       exchange_rate_usd, error_message, notes, created_at
		FROM statements ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStatements(rows)
}

// ListPending returns statements with status=pending.
func ListPendingStatements(db *sql.DB) ([]*domain.Statement, error) {
	rows, err := db.Query(`
		SELECT id, source_file, file_hash, card_type, card_bank, statement_period,
		       parser_id, status, processed_at, total_txns, total_amount_ars,
		       exchange_rate_usd, error_message, notes, created_at
		FROM statements WHERE status = 'pending' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanStatements(rows)
}

// UpdateProcessed marks a statement as processed with final metadata.
func UpdateProcessed(db *sql.DB, id int64, cardType, cardBank, period, parserID string, totalTxns int, totalARS float64, exchangeRate *float64) error {
	now := time.Now()
	_, err := db.Exec(`
		UPDATE statements SET
			card_type = ?, card_bank = ?, statement_period = ?, parser_id = ?,
			status = 'reviewed', processed_at = ?, total_txns = ?,
			total_amount_ars = ?, exchange_rate_usd = ?, error_message = ''
		WHERE id = ?`,
		cardType, cardBank, period, parserID, now, totalTxns, totalARS, exchangeRate, id)
	return err
}

// UpdateError marks a statement as failed with an error message.
func UpdateError(db *sql.DB, id int64, errMsg string) error {
	_, err := db.Exec(`UPDATE statements SET status = 'error', error_message = ? WHERE id = ?`, errMsg, id)
	return err
}

// FileHashExists returns true if any statement with this hash has been fully processed.
func FileHashExists(db *sql.DB, hash string) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM statements WHERE file_hash = ? AND status = 'reviewed'`, hash).Scan(&count)
	return count > 0, err
}

func scanStatement(row *sql.Row) (*domain.Statement, error) {
	s := &domain.Statement{}
	var processedAt sql.NullString
	var exchangeRate sql.NullFloat64
	err := row.Scan(
		&s.ID, &s.SourceFile, &s.FileHash, &s.CardType, &s.CardBank,
		&s.StatementPeriod, &s.ParserID, &s.Status, &processedAt,
		&s.TotalTxns, &s.TotalAmountARS, &exchangeRate,
		&s.ErrorMessage, &s.Notes, &s.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan statement: %w", err)
	}
	if processedAt.Valid {
		t, _ := time.Parse("2006-01-02T15:04:05Z07:00", processedAt.String)
		s.ProcessedAt = &t
	}
	if exchangeRate.Valid {
		s.ExchangeRateUSD = &exchangeRate.Float64
	}
	return s, nil
}

func scanStatements(rows *sql.Rows) ([]*domain.Statement, error) {
	var result []*domain.Statement
	for rows.Next() {
		s := &domain.Statement{}
		var processedAt sql.NullString
		var exchangeRate sql.NullFloat64
		err := rows.Scan(
			&s.ID, &s.SourceFile, &s.FileHash, &s.CardType, &s.CardBank,
			&s.StatementPeriod, &s.ParserID, &s.Status, &processedAt,
			&s.TotalTxns, &s.TotalAmountARS, &exchangeRate,
			&s.ErrorMessage, &s.Notes, &s.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan statement row: %w", err)
		}
		if processedAt.Valid {
			t, _ := time.Parse("2006-01-02T15:04:05Z07:00", processedAt.String)
			s.ProcessedAt = &t
		}
		if exchangeRate.Valid {
			s.ExchangeRateUSD = &exchangeRate.Float64
		}
		result = append(result, s)
	}
	return result, rows.Err()
}
