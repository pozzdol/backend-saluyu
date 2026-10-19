package handler

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/pozzdol/backend-saluyu/internal/service"
)

// apiError gives every known service error an HTTP status and a stable code. The frontend
// translates the code; "error" keeps the English text for logs and old clients.
type apiError struct {
	err    error
	status int
	code   string
}

var apiErrors = []apiError{
	{service.ErrEmailTaken, http.StatusConflict, "email_taken"},
	{service.ErrInvalidCredentials, http.StatusUnauthorized, "invalid_credentials"},
	{service.ErrTokenRevoked, http.StatusUnauthorized, "token_revoked"},
	{service.ErrSessionExpired, http.StatusUnauthorized, "session_expired"},
	{service.ErrUserInactive, http.StatusForbidden, "user_inactive"},
	// 400, not 401: the token is fine, so the client must not log the user out.
	{service.ErrWrongPassword, http.StatusBadRequest, "wrong_password"},
	{service.ErrPasswordTooLong, http.StatusBadRequest, "password_too_long"},

	{service.ErrUserNotFound, http.StatusNotFound, "not_found"},
	{service.ErrAccountNotFound, http.StatusNotFound, "account_not_found"},
	{service.ErrEntryNotFound, http.StatusNotFound, "entry_not_found"},
	{service.ErrContactNotFound, http.StatusNotFound, "contact_not_found"},
	{service.ErrNotFound, http.StatusNotFound, "not_found"},

	{service.ErrAccountCodeTaken, http.StatusConflict, "account_code_taken"},
	{service.ErrNotOpened, http.StatusConflict, "books_not_opened"},
	{service.ErrBooksNotEmpty, http.StatusConflict, "books_not_empty"},
	{service.ErrOpeningLocked, http.StatusConflict, "opening_locked"},
	{service.ErrContactNameTaken, http.StatusConflict, "contact_name_taken"},
	{service.ErrContactHasBalance, http.StatusConflict, "contact_has_balance"},
	{service.ErrContactHasHistory, http.StatusConflict, "contact_has_history"},
	{service.ErrOverpayment, http.StatusConflict, "overpayment"},
	{service.ErrLoanNotPending, http.StatusConflict, "already_answered"},
	{service.ErrEntryLocked, http.StatusConflict, "entry_locked"},
	{service.ErrNotShared, http.StatusConflict, "not_shared"},
	{service.ErrAlreadyLinked, http.StatusConflict, "already_linked"},

	{service.ErrSameAccount, http.StatusBadRequest, "same_account"},
	{service.ErrWrongAccountType, http.StatusBadRequest, "wrong_account_type"},
	{service.ErrAccountInactive, http.StatusBadRequest, "account_inactive"},
	{service.ErrBeforeStart, http.StatusBadRequest, "before_start"},
	{service.ErrLockInFuture, http.StatusBadRequest, "lock_in_future"},
	{service.ErrNotBalanceSheet, http.StatusBadRequest, "not_balance_sheet"},
	{service.ErrNeedsContact, http.StatusBadRequest, "needs_contact"},
	{service.ErrDuplicateAccount, http.StatusBadRequest, "duplicate_account"},
	{service.ErrWrongCounter, http.StatusBadRequest, "wrong_counter"},
	{service.ErrNotExpenseAccount, http.StatusBadRequest, "not_expense_account"},
	{service.ErrContactNotAllowed, http.StatusBadRequest, "contact_not_allowed"},
	{service.ErrNeedsExpense, http.StatusBadRequest, "needs_expense"},
	{service.ErrWrongMirror, http.StatusBadRequest, "wrong_mirror"},
	{service.ErrWrongFee, http.StatusBadRequest, "wrong_fee"},
	{service.ErrKindChange, http.StatusBadRequest, "kind_change"},
	{service.ErrNotFixedAsset, http.StatusBadRequest, "not_fixed_asset"},
	{service.ErrNoValueChange, http.StatusBadRequest, "no_value_change"},
	// Forms always build balanced entries; only the line editor can send unbalanced ones.
	{service.ErrUnbalanced, http.StatusBadRequest, "unbalanced"},
	{service.ErrWrongEditForm, http.StatusBadRequest, "wrong_edit_form"},
	{service.ErrMirrorLocked, http.StatusConflict, "mirror_locked"},
	{service.ErrStartAfterEntries, http.StatusConflict, "start_after_entries"},

	{service.ErrNotACard, http.StatusNotFound, "not_a_card"},
	{service.ErrPlanNotFound, http.StatusNotFound, "plan_not_found"},
	{service.ErrAlreadyPaid, http.StatusConflict, "already_paid"},
	{service.ErrPlanHasPayments, http.StatusConflict, "plan_has_payments"},

	{service.ErrRuleNotFound, http.StatusNotFound, "rule_not_found"},
	{service.ErrRuleMissing, http.StatusNotFound, "rule_not_found"},
	{service.ErrOccurrenceHandled, http.StatusConflict, "occurrence_handled"},
	{service.ErrNotDue, http.StatusConflict, "not_due"},
	{service.ErrNeedsDay, http.StatusBadRequest, "needs_day"},
	{service.ErrUnreadableFile, http.StatusBadRequest, "unreadable_file"},
	{service.ErrNoColumns, http.StatusBadRequest, "no_columns"},
	{service.ErrImportAccount, http.StatusBadRequest, "import_account"},

	{service.ErrGoalNotFound, http.StatusNotFound, "goal_not_found"},
	{service.ErrInvalidInvite, http.StatusBadRequest, "invalid_invite"},
	{service.ErrAlreadyInGroup, http.StatusConflict, "already_in_group"},
	{service.ErrPeriodLocked, http.StatusConflict, "period_locked"},
	{service.ErrUnknownReport, http.StatusBadRequest, "unknown_report"},

	{service.ErrInvalidLink, http.StatusBadRequest, "invalid_link"},
	{service.ErrMFAInvalid, http.StatusUnauthorized, "mfa_expired"},
	{service.ErrInvalidCode, http.StatusBadRequest, "invalid_code"},
	{service.ErrTOTPNotSetup, http.StatusConflict, "totp_not_setup"},
	{service.ErrTOTPAlreadyEnabled, http.StatusConflict, "totp_already_enabled"},
	{service.ErrSessionNotFound, http.StatusNotFound, "session_not_found"},
	{service.ErrEmailUnverified, http.StatusForbidden, "email_unverified"},
}

// fail maps service errors to HTTP responses in one place.
// Unknown errors are logged and hidden behind a generic 500.
func fail(c *gin.Context, op string, err error) {
	for _, e := range apiErrors {
		if errors.Is(err, e.err) {
			msg := err.Error()
			if e.err == service.ErrPasswordTooLong {
				msg = "password must be at most 72 bytes"
			}
			c.JSON(e.status, gin.H{"error": msg, "code": e.code})
			return
		}
	}
	log.Printf("failed to %s: %v", op, err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error", "code": "internal"})
}
