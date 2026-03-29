-- Rename card_type 'MASTER' → 'MASTERCARD' for existing rows.
UPDATE transactions SET card_type = 'MASTERCARD' WHERE card_type = 'MASTER';
UPDATE statements   SET card_type = 'MASTERCARD' WHERE card_type = 'MASTER';

-- Rename parser_id for statements processed with the old package names.
UPDATE statements SET parser_id = 'galicia_mastercard'     WHERE parser_id = 'galicia_master';
UPDATE statements SET parser_id = 'galicia_mastercard_mas' WHERE parser_id = 'galicia_master_mas';
