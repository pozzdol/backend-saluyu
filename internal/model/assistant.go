package model

// AssistantRequest is the whole chat so far; the server keeps no state. Model turns are the
// earlier AssistantReply JSON, sent back as they came, so the AI sees the draft it is revising.
type AssistantRequest struct {
	Today    string        `json:"today" binding:"required,datetime=2006-01-02"` // the user's local date
	Messages []ChatMessage `json:"messages" binding:"required,min=1,max=30,dive"`
}

type ChatMessage struct {
	Role string `json:"role" binding:"required,oneof=user model"`
	Text string `json:"text" binding:"required,max=8000"`
}

// AssistantReply is a draft the user still saves. Amounts are rupiah, as the AI reads and writes
// them; the client converts to sen when posting.
type AssistantReply struct {
	Reply string `json:"reply"`
	// Save: the user asked to save and every item is complete.
	Save        bool         `json:"save"`
	Items       []DraftItem  `json:"items"`
	NewAccounts []NewAccount `json:"new_accounts"`
}

// DraftItem is one entry (POST /entries) or one loan (POST /loans). Account and contact fields
// hold an id, "new:<name>" for one the user agreed to create, or "" when unknown.
type DraftItem struct {
	Target         string  `json:"target"`    // entry or loan
	Kind           string  `json:"kind"`      // entry: income, expense, transfer
	LoanType       string  `json:"loan_type"` // loan: talangan, pinjam, lunas_masuk, lunas_keluar
	Amount         float64 `json:"amount"`
	EntryDate      string  `json:"entry_date"`
	Description    string  `json:"description"`
	From           string  `json:"from"`
	To             string  `json:"to"`
	NextPeriod     bool    `json:"next_period"`
	Account        string  `json:"account"` // loan: the money account
	Contact        string  `json:"contact"`
	MyShare        float64 `json:"my_share"`
	ExpenseAccount string  `json:"expense_account"`
	ConfirmOverpay bool    `json:"confirm_overpay"`
}

type NewAccount struct {
	Name string `json:"name"`
	Type string `json:"type"` // Asset, Liability, Revenue or Expense
	Icon string `json:"icon"`
}
