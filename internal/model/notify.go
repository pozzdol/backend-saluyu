package model

import "github.com/google/uuid"

// NotifyPrefs says which email reminders a user wants; all on by default.
type NotifyPrefs struct {
	DueBills bool `json:"due_bills"`
	Budget   bool `json:"budget"`
	Weekly   bool `json:"weekly"`
	Inactive bool `json:"inactive"`
}

// NotifyPrefsRequest replaces all four switches; a pointer so that false is not "missing".
type NotifyPrefsRequest struct {
	DueBills *bool `json:"due_bills" binding:"required"`
	Budget   *bool `json:"budget" binding:"required"`
	Weekly   *bool `json:"weekly" binding:"required"`
	Inactive *bool `json:"inactive" binding:"required"`
}

// NotifyUser is a user the reminder job may write to: active, with a verified email.
type NotifyUser struct {
	ID     uuid.UUID
	Name   string
	Email  string
	Locale *string
	Prefs  NotifyPrefs
}
