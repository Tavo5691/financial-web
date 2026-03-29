package domain

import "time"

// Transaction represents a single credit card charge or credit.
type Transaction struct {
	ID                      string
	StatementID             int64
	StatementPeriod         string // YYYY-MM
	CardType                string // VISA | MASTERCARD
	CardBank                string // GALICIA | UALA | BBVA
	TransactionDate         time.Time
	DescriptionOriginal     string
	Merchant                string
	AliasID                 *int64
	Category                string
	Amount                  float64
	Currency                string // ARS | USD
	AmountARS               float64
	ExchangeRate            *float64
	IsInstallment           bool
	InstallmentCurrent      int
	InstallmentTotal        int
	InstallmentGroupID      string
	InstallmentOriginalAmount *float64
	ExpenseType             string // fixed | variable | installment
	IsDebit                 bool
	IsHidden                bool
	Notes                   string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// Statement represents a processed PDF statement.
type Statement struct {
	ID              int64
	SourceFile      string
	FileHash        string
	CardType        string
	CardBank        string
	StatementPeriod string // YYYY-MM
	ParserID        string
	Status          string // pending | processing | reviewed | error
	ProcessedAt     *time.Time
	TotalTxns       int
	TotalAmountARS  float64
	ExchangeRateUSD *float64
	ErrorMessage    string
	Notes           string
	CreatedAt       time.Time
}

// Alias maps a description pattern to a normalized merchant name and category.
type Alias struct {
	ID          int64
	Pattern     string // stored uppercase; matched with LIKE '%pattern%'
	Merchant    string
	Category    string
	ExpenseType string
	Notes       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Category defines an expense category with optional keyword hints.
type Category struct {
	Name               string
	DefaultExpenseType string
	Keywords           string // comma-separated
	SortOrder          int
}

// StatementStatus constants.
const (
	StatusPending    = "pending"
	StatusProcessing = "processing"
	StatusReviewed   = "reviewed"
	StatusError      = "error"
)

// CardType constants.
const (
	CardVisa   = "VISA"
	CardMaster = "MASTERCARD"
)

// Currency constants.
const (
	CurrencyARS = "ARS"
	CurrencyUSD = "USD"
)

// ExpenseType constants.
const (
	ExpenseFixed       = "fixed"
	ExpenseVariable    = "variable"
	ExpenseInstallment = "installment"
)
