package models

import "time"

type Position struct {
	ID             string       `json:"id"`
	Symbol         string       `json:"symbol"`
	PutStrike      float64      `json:"put_strike"`
	CallStrike     float64      `json:"call_strike"`
	Expiration     time.Time    `json:"expiration"`
	Quantity       int          `json:"quantity"`
	CreditReceived float64      `json:"credit_received"`
	EntryDate      time.Time    `json:"entry_date"`
	EntryIVR       float64      `json:"entry_ivr"`
	EntrySpot      float64      `json:"entry_spot"`
	Status         string       `json:"status"` // "open", "closed", "adjusted"
	CurrentPnL     float64      `json:"current_pnl"`
	DTE            int          `json:"dte"`
	Adjustments    []Adjustment `json:"adjustments"`
}

type Adjustment struct {
	Date        time.Time `json:"date"`
	Type        string    `json:"type"` // "roll_put", "roll_call", "straddle", "inverted"
	OldStrike   float64   `json:"old_strike"`
	NewStrike   float64   `json:"new_strike"`
	Credit      float64   `json:"credit"`
	Description string    `json:"description"`
}

func (p *Position) CalculateDTE() int {
	return int(time.Until(p.Expiration).Hours() / 24)
}

func (p *Position) GetTotalCredit() float64 {
	total := p.CreditReceived
	for _, adj := range p.Adjustments {
		total += adj.Credit
	}
	return total
}

func (p *Position) ProfitPercent() float64 {
	if p.CreditReceived == 0 {
		return 0
	}
	return (p.CurrentPnL / p.CreditReceived) * 100
}