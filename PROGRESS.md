# Progress

_Agents: update this file after completing significant work. Mark items done, add new ones, keep "Current state" accurate._

---

## Current state (2026-03-30)

All 4 active parsers validated and live. `galicia_mastercard_mas` removed (will not be implemented). DB reset, Categories CRUD, two report pages, and transaction filter improvements all live.

To verify the app is working:
```bash
go run ./cmd/server -db ./data/financial.db -inbox ./inbox
# → http://localhost:8080
# /aliases — 95 aliases
# /categories — 18 categories with full CRUD
# /transactions — filterable by period, Card+Bank combo, category (A→Z), free text
# /reports/monthly — category totals with drill-down; defaults to most recent period
# /reports/installments — active cuota plans + month-by-month projection calendar
```

---

## Completed

### Phase 1 — Foundation
- [x] Go module (`financial-web`)
- [x] Dependencies: `modernc.org/sqlite` (pure Go SQLite), `github.com/ledongthuc/pdf`
- [x] SQLite schema: `statements`, `transactions`, `aliases`, `categories` tables
- [x] Idempotent migrations (001_initial.sql, 002_indexes.sql) run on startup
- [x] `internal/domain/types.go` — shared structs + constants
- [x] DB query layer: `queries/transactions.go`, `queries/statements.go`, `queries/aliases.go`

### Phase 2 — Parser infrastructure + validate CLI
- [x] `internal/parser/interface.go` — `Parser` interface, `ParsedTransaction`, `StatementMeta`
- [x] `internal/parser/registry.go` — `Register()`, `Get()`, `All()`
- [x] `internal/pdf/extractor.go` — PDF → raw text wrapper
- [x] 5 parser packages registered (4 still stubs):
  - `internal/parser/galicia_master/` — stub
  - `internal/parser/galicia_master_mas/` — stub
  - `internal/parser/uala_master/` — stub
  - `internal/parser/bbva_visa/` — stub
- [x] `cmd/validate/main.go` — CLI with `-list`, `-extract`, `-expected`, `-verbose` flags

### Phase 3 — Migration (CSV → SQLite)
- [x] `cmd/migrate/main.go`
- [x] **Migration already run**: 18 categories, 95 aliases, 5 statements, 288 transactions imported from `/home/gustavo/Code/financial/`

### Phase 4 — Web app
- [x] `internal/watcher/watcher.go` — 5s polling, SHA256 dedup, INSERT OR IGNORE
- [x] `internal/categorizer/categorizer.go` — alias match → keyword fallback → "Otros"
- [x] `internal/service/statement_service.go` — orchestrates parse → categorize → store
- [x] `internal/service/alias_service.go` — CRUD + retroactive UPDATE on aliases
- [x] Per-page isolated template sets (avoids Go `{{define}}` global namespace collision)
- [x] `static/htmx.min.js` vendored (v1.9.12), `static/style.css` dark theme
- [x] All routes wired:
  - `/inbox` — lists PDFs, Actualizar button, Procesar modal
  - `/inbox/scan` — HTMX partial rescan
  - `/inbox/{id}/process-form` — parser picker modal
  - `/inbox/process` — triggers processing, redirects to review
  - `/statements/{id}/review` — all transactions for a statement, inline edit
  - `/transactions` — filterable list (period, card, category, search)
  - `/transactions/{id}/edit|view|PATCH|/hide` — inline editing + hide toggle
  - `/aliases` — CRUD with retroactive propagation + toast feedback
  - `/categories` — CRUD with rename cascade (aliases + transactions), delete blocked if in use
  - `/parsers` — static list of registered parsers
- [x] `cmd/server/main.go` — graceful shutdown, signal handling
- [x] `.gitignore` — excludes `data/`, `inbox/`, `samples/`

### Phase 4b — Parser implementations

- [x] `galicia_visa` parser — handles multi-page, multi-card, installments, no-ref USD/refund transactions; validated on enero/febrero/marzo 2026 (117 / 78 / 83 txns)
- [x] `uala_mastercard` parser — hybrid plain-text + positioned-text approach; detects ARS vs USD by column X coordinate; skips RG 5463 perception (nets to zero); validated on enero/febrero 2026 (38 / 27 txns); renamed from `uala_master`
- [x] `bbva_visa` parser — XObject-based PDF; uses pdfcpu token extraction; aggregates IIBB + IVA; handles USD; validated on enero/febrero/marzo 2026 (98 / 86 / 105 txns)
- [x] `galicia_mastercard` parser — validated on enero/febrero/marzo 2026 (6 / 4 / 2 txns)
- `galicia_mastercard_mas` — removed; will not be implemented

### Phase 4c — UI enhancements

- [x] Categories CRUD (`/categories`) — create, edit (with rename cascade), delete (blocked if in use)
- [x] Transactions page: Card filter now uses combined Card+Bank format (Visa Galicia, Visa BBVA, Mastercard Galicia, Mastercard Ualá); categories dropdown sorted A→Z
- [x] `TransactionFilter.ExpenseType` field added; used in report and drill-down queries
- [x] Monthly report category expand/collapse toggle — fixed via `hx-on::before-request` / `hx-on::after-request` inline on the button (replaced unreliable global `htmx:afterSwap` listener)

---

## Known issues

- No known issues with parsers. All 4 active parsers validated.

---

## Next steps

### Fix ID sequencing bug

- [x] Fixed ID sequencing bug in `statement_service.go` — sequence is now computed once before the loop; `seq` counter incremented locally per transaction
- [x] Fixed related bug: `InsertTransaction` now accepts an `Execer` interface (`*sql.DB` or `*sql.Tx`), so batch inserts run inside a real DB transaction with proper rollback on error

### Phase 5 — Reports

These map to the existing Python scripts in `/home/gustavo/Code/financial/reports/`:

- [x] Monthly summary (`/reports/monthly`) — totals by category with Fijo/Variable/Cuotas breakdown, filterable by period + card + expense type, HTMX drill-down per category
- [ ] Category breakdown — fixed / variable / installment split
- [x] Debt tracker (`/reports/installments`) — active installment plans with remaining count, monthly amount, final month, total pending; month-by-month forward projection calendar
- [ ] Month comparison — trend view with % change month-over-month
- [ ] Top merchants — ranked spending by merchant

### Future ideas

- [ ] Export transactions as CSV from the UI
- [ ] Duplicate detection when processing a PDF (same amount + date + description already in DB)
- [ ] Bulk alias creation from uncategorized transaction list
- [ ] Dark/light theme toggle
