CREATE TABLE IF NOT EXISTS statements (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    source_file      TEXT    NOT NULL UNIQUE,
    file_hash        TEXT    NOT NULL,
    card_type        TEXT    NOT NULL,
    card_bank        TEXT    NOT NULL,
    statement_period TEXT    NOT NULL,
    parser_id        TEXT    NOT NULL DEFAULT '',
    status           TEXT    NOT NULL DEFAULT 'pending',
    processed_at     DATETIME,
    total_txns       INTEGER NOT NULL DEFAULT 0,
    total_amount_ars REAL    NOT NULL DEFAULT 0,
    exchange_rate_usd REAL,
    error_message    TEXT    NOT NULL DEFAULT '',
    notes            TEXT    NOT NULL DEFAULT '',
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS aliases (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    pattern      TEXT NOT NULL UNIQUE,
    merchant     TEXT NOT NULL,
    category     TEXT NOT NULL,
    expense_type TEXT NOT NULL DEFAULT 'variable',
    notes        TEXT NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS categories (
    name                 TEXT PRIMARY KEY,
    default_expense_type TEXT NOT NULL DEFAULT 'variable',
    keywords             TEXT NOT NULL DEFAULT '',
    sort_order           INTEGER NOT NULL DEFAULT 99
);

CREATE TABLE IF NOT EXISTS transactions (
    id                          TEXT PRIMARY KEY,
    statement_id                INTEGER NOT NULL REFERENCES statements(id),
    statement_period            TEXT    NOT NULL,
    card_type                   TEXT    NOT NULL,
    card_bank                   TEXT    NOT NULL,
    transaction_date            TEXT    NOT NULL,
    description_original        TEXT    NOT NULL,
    merchant                    TEXT    NOT NULL DEFAULT '',
    alias_id                    INTEGER REFERENCES aliases(id),
    category                    TEXT    NOT NULL,
    amount                      REAL    NOT NULL,
    currency                    TEXT    NOT NULL DEFAULT 'ARS',
    amount_ars                  REAL    NOT NULL,
    exchange_rate               REAL,
    is_installment              INTEGER NOT NULL DEFAULT 0,
    installment_current         INTEGER NOT NULL DEFAULT 0,
    installment_total           INTEGER NOT NULL DEFAULT 0,
    installment_group_id        TEXT    NOT NULL DEFAULT '',
    installment_original_amount REAL,
    expense_type                TEXT    NOT NULL DEFAULT 'variable',
    is_debit                    INTEGER NOT NULL DEFAULT 1,
    is_hidden                   INTEGER NOT NULL DEFAULT 0,
    notes                       TEXT    NOT NULL DEFAULT '',
    created_at                  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT OR IGNORE INTO categories (name, default_expense_type, sort_order) VALUES
    ('Supermercado',         'variable',    1),
    ('Restaurantes y Bares', 'variable',    2),
    ('Delivery',             'variable',    3),
    ('Transporte',           'variable',    4),
    ('Servicios Digitales',  'fixed',       5),
    ('Telefonia e Internet', 'fixed',       6),
    ('Salud',                'variable',    7),
    ('Indumentaria',         'variable',    8),
    ('Hogar',                'variable',    9),
    ('Hogar Cuotas',         'installment', 10),
    ('Gimnasio',             'fixed',       11),
    ('Entretenimiento',      'variable',    12),
    ('Educacion',            'variable',    13),
    ('Seguros',              'fixed',       14),
    ('Impuestos y Tasas',    'fixed',       15),
    ('Viajes',               'variable',    16),
    ('Compras Online',       'variable',    17),
    ('Otros',                'variable',    18);
