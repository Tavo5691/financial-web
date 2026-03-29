# financial-web

A personal finance web app for processing Argentine credit card PDF statements, built as a local-first tool for household use.

## What it does

Every month, Argentine banks send credit card statements as PDFs. This app:

1. **Watches a folder** for new PDF statements dropped in
2. **Parses them** into individual transactions using bank-specific parsers
3. **Categorizes** each transaction automatically via alias rules and keyword matching
4. **Presents reports**: monthly totals by category, installment plan tracker, transaction search and editing

All data stays local — there is no server, no cloud, no account. The app runs on `localhost`.

## Tech stack

| Layer | Choice | Why |
|-------|--------|-----|
| Language | Go 1.25 | Single binary, fast startup, strong stdlib |
| Database | SQLite (`modernc.org/sqlite`) | Pure Go, no CGo, single file, zero ops |
| Frontend | HTMX + server-rendered HTML | No build step, no JS framework, fast enough |
| PDF extraction | `ledongthuc/pdf` + `pdfcpu` | Different banks use different PDF structures |
| Sync | Git + GitHub (private repo) | Simplest possible multi-device sync |

No Docker. No ORM. No frontend build pipeline. The only runtime dependency is a Go toolchain.

## Architecture

```
cmd/
  server/   — HTTP server; blank-imports all parser packages to trigger init() registration
  validate/ — CLI for parser development (-extract, -expected, -verbose)
  migrate/  — one-time CSV → SQLite importer (historical data)
internal/
  parser/   — Parser interface + registry + one package per bank
  pdf/      — PDF text extractor (ledongthuc/pdf) + token extractor (pdfcpu)
  watcher/  — 5s polling loop for inbox/; SHA-256 dedup
  service/  — StatementService (parse → categorize → store), AliasService
  web/      — HTTP handlers + Go html/template pages
  db/       — SQL migrations + query layer (no ORM)
```

The template system builds **one isolated `*template.Template` per page** to avoid Go's global `{{define}}` namespace collisions across pages.

## Parser system

Each bank parser lives in its own package and registers itself via `init()`:

```go
func init() {
    parser.Register(&galiciaVisaParser{})
}
```

The server and CLI blank-import all parser packages — adding a new bank requires creating one package and two import lines. Parsers receive raw PDF text (or, for XObject-based PDFs like BBVA, a flat token stream) and return structured transactions with no DB access.

Parsers implemented: `galicia_visa`, `bbva_visa`
Parsers stubbed: `galicia_mastercard`, `galicia_mastercard_mas`, `uala_master`

## AI-assisted development

This project was built using [Claude Code](https://claude.ai/code) (Anthropic's agentic CLI) as the primary development tool, with the human in a directing role.

The most interesting application was **parser development**. Argentine bank PDFs are messy — pages with split Unicode characters, XObject form streams instead of content streams, amounts embedded inside description fields. Each parser required:

- Extracting raw tokens from the PDF to understand the structure
- Writing diagnostic scripts to map token layout across pages
- Designing a state machine to walk the token stream
- Iterating until the transaction count matched expected values

Claude Code handled all of this end-to-end: writing the diagnostic tool, reading its output, identifying patterns, implementing the parser, running `go run ./cmd/validate -verbose`, and fixing edge cases — without human intervention between steps. The human provided the sample PDFs, defined the business rules (e.g. "ignore RG 5617 rows", "aggregate IIBB lines into one transaction"), and approved the approach.

The overall architecture, schema design, and UI patterns were also designed collaboratively through plan-then-implement cycles.

## Running locally

```bash
# Start the server
./start.sh

# Or directly:
go run ./cmd/server -db ./data/financial.db -inbox ./inbox -port 8080

# Validate a parser against a sample PDF
go run ./cmd/validate -expected 105 -verbose bbva_visa samples/bbva_visa_marzo2026.pdf
```

## Sync between computers

The `start.sh` script wraps the server with git pull/push so the database stays in sync across two machines via a private GitHub repo:

```bash
./start.sh  # pulls latest DB → starts server → commits + pushes on Ctrl+C
```
