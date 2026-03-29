CREATE INDEX IF NOT EXISTS idx_transactions_statement_id  ON transactions(statement_id);
CREATE INDEX IF NOT EXISTS idx_transactions_period         ON transactions(statement_period);
CREATE INDEX IF NOT EXISTS idx_transactions_card           ON transactions(card_type, card_bank);
CREATE INDEX IF NOT EXISTS idx_transactions_is_hidden      ON transactions(is_hidden);
CREATE INDEX IF NOT EXISTS idx_transactions_alias_id       ON transactions(alias_id);
CREATE INDEX IF NOT EXISTS idx_transactions_description    ON transactions(description_original);
CREATE INDEX IF NOT EXISTS idx_aliases_pattern             ON aliases(pattern);
CREATE INDEX IF NOT EXISTS idx_statements_status           ON statements(status);
CREATE INDEX IF NOT EXISTS idx_statements_period           ON statements(statement_period);
