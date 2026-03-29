# Financial Web — Claude Code Guide

## What this is

A local single-user web app for processing Argentine credit card PDF statements.
Go + HTMX + SQLite. No auth. No Docker required.

## Running the project

```bash
# Start the server
go run ./cmd/server -db ./data/financial.db -inbox ./inbox -port 8080

# Validate a parser (see Parser Development below)
go run ./cmd/validate -list
go run ./cmd/validate -extract samples/some.pdf
go run ./cmd/validate -expected 126 galicia_visa samples/galicia_visa_enero.pdf

# One-time CSV migration (already done — don't run again unless DB is reset)
go run ./cmd/migrate --transactions ../financial/data/transactions.csv \
  --statements ../financial/data/statements_log.csv \
  --merchants  ../financial/config/merchants.csv \
  --categories ../financial/config/categories.csv \
  --db ./data/financial.db
```

## Architecture in one paragraph

The server (`cmd/server`) blank-imports all parser packages to trigger their `init()` registration, then starts an HTTP server using stdlib `net/http` (Go 1.22+ with `{id}` path params). A background goroutine polls `inbox/` every 5 seconds for new PDFs and inserts them as `pending` statements. All pages are server-rendered using `html/template` with HTMX for partial updates. SQLite is accessed directly via `database/sql` with `modernc.org/sqlite` (pure Go, no CGo).

## Project layout

```
cmd/
  server/   — web server entrypoint; blank-imports all parsers
  validate/ — CLI for parser development and testing
  migrate/  — one-time CSV → SQLite importer
internal/
  db/
    migrations/  — SQL files run in order on startup (idempotent)
    queries/     — SQL functions (no ORM)
  domain/        — shared structs (Transaction, Statement, Alias, Category)
  parser/        — Parser interface + registry + one package per bank
  pdf/           — PDF text extractor (ledongthuc/pdf wrapper)
  categorizer/   — alias lookup → keyword fallback → "Otros"
  watcher/       — 5s polling watcher for inbox/
  service/       — business logic (StatementService, AliasService)
  web/           — HTTP handlers, templates, template rendering
    templates/
      partials/  — HTMX fragments (define names end in .html)
      *.html     — full pages (define "title" + "content" blocks)
static/          — htmx.min.js (vendored), style.css
samples/         — PDFs for parser development (not watched)
inbox/           — watched folder for new statements
data/            — financial.db lives here
```

## Key architectural decisions

### Template system
Go's `{{define "content"}}` blocks are global within a template set, so multiple pages would clobber each other. Instead, `server.go` builds **one isolated `*template.Template` per page** (layout + partials + the specific page). The `render()` function selects the right set and calls `ExecuteTemplate("layout.html", data)`. Partials (HTMX fragments) share a separate set and are executed by their define name.

Do NOT go back to a single shared template set.

### Parser registration
Each parser package calls `parser.Register(&myParser{})` in its `init()`. The server and validate CLI blank-import all parser packages. Adding a new parser = create the package + add one blank import line to both `cmd/server/main.go` and `cmd/validate/main.go` + rebuild.

### Alias retroactivity
When an alias is created or updated, `alias_service.go` runs a single SQL UPDATE matching `UPPER(description_original) LIKE '%' || UPPER(pattern) || '%'`. This is synchronous and fast at current scale (~300–500 rows). Do not move it to a background job unless the transaction count grows significantly.

### SQLite
- Single writer: `db.SetMaxOpenConns(1)`
- WAL mode + foreign keys: set in the DSN string in `db.Open()`
- All boolean columns are `INTEGER 0/1` (SQLite has no native bool)
- All date columns are `TEXT "YYYY-MM-DD"` (scanned as string, parsed in Go)

## Parser development workflow

When the user brings a PDF from a new bank:

1. Copy the PDF to `samples/`
2. Extract raw text to understand the format:
   ```bash
   go run ./cmd/validate -extract samples/new_bank.pdf > /tmp/new_bank.txt
   ```
3. Analyze the text and write `internal/parser/new_bank/parser.go` implementing the `parser.Parser` interface
4. Add a blank import to `cmd/server/main.go` and `cmd/validate/main.go`
5. Validate until PASS:
   ```bash
   go run ./cmd/validate -expected N -verbose new_bank samples/new_bank.pdf
   ```
6. Rebuild the server — the parser appears in the process dropdown

### Parser interface

```go
type Parser interface {
    ID()   string       // stable, used as FK in statements table
    Info() ParserInfo
    Parse(text string) (StatementMeta, []ParsedTransaction, error)
}
```

`Parse` receives the full PDF text (all pages joined). It must NOT touch the database.

### Common parsing patterns in Argentine bank statements

- Transaction lines typically contain: date, description, amount (possibly with cuotas notation like `2/6`)
- USD amounts appear alongside ARS amounts with an exchange rate line somewhere
- IVA, percepciones, impuesto PAIS appear as separate line items — include them as transactions
- Installments: parse `N/M` notation → set `IsInstallment=true`, `InstallmentCurrent=N`, `InstallmentTotal=M`
- Credits (pagos, devoluciones) have `IsDebit=false`

## HTMX patterns used

- `hx-get/post/patch/delete` on forms and buttons
- `hx-target="#element-id"` + `hx-swap="outerHTML"` for row replacement
- `hx-swap="innerHTML"` for list refresh
- `hx-push-url="true"` on filter forms so URL stays bookmarkable
- `hx-confirm` on destructive actions (hide, delete)
- Toast notifications via `HX-Trigger: {"showToast": {"msg":"...", "type":"success"}}` response header — the JS listener in `layout.html` handles this

## Updating PROGRESS.md

After completing any significant work, update `PROGRESS.md`:
- Mark completed items with `[x]`
- Add new items to the appropriate section
- Note any known bugs or regressions discovered
- Keep the "Current state" section accurate

## What NOT to do

- Do not add external dependencies without a strong reason — stdlib + the two declared deps cover almost everything
- Do not use a single shared template set (see Template system above)
- Do not run the migration again on an existing populated DB
- Do not use `is_hidden = 1` transactions in any report or aggregate query — always filter `WHERE is_hidden = 0`
- Do not store exchange rates in a separate table — they are denormalized per transaction intentionally
- Do not implement authentication — this is a local single-user app
