// Package event описывает биллинговое событие, которое приходит от платёжной системы.
package event

import "time"

// Type — тип биллингового события.
type Type string

const (
	TypePayment Type = "billing-payment"
	TypeRefund  Type = "billing-refund"
	TypeCreated Type = "billing-created"
)

// Event — биллинговое событие от платёжной системы (PaymentCore).
type Event struct {
	ID        string    `json:"id"`
	ProjectID int       `json:"project_id"`
	UserID    string    `json:"user_id"` // CUID: SHA256 от email пользователя
	Type      Type      `json:"type"`
	DryRun    int       `json:"dry_run"` // 0 или 1
	Timestamp time.Time `json:"timestamp"`
	Data      Billing   `json:"data"`
}

// Billing — платёжная часть события.
type Billing struct {
	InvoiceID int64   `json:"invoice_id"`
	Amount    float64 `json:"amount"`
	Currency  string  `json:"currency"`
}
