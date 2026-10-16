package service

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/recurring"
	"github.com/pozzdol/backend-saluyu/internal/repository"
)

var (
	ErrRuleNotFound      = repository.ErrRuleNotFound
	ErrOccurrenceHandled = repository.ErrOccurrenceHandled
	ErrNotDue            = errors.New("this date is not an open occurrence of the rule")
	ErrNeedsDay          = errors.New("weekly rules need a weekday (0-6), monthly rules a day (1-31)")
)

type RecurringRepository interface {
	PeriodStartDay(ctx context.Context, userID uuid.UUID) (int, error)
	SaveRule(ctx context.Context, rule *model.RecurringRule) error
	DeleteRule(ctx context.Context, userID, id uuid.UUID) error
	Rules(ctx context.Context, userID uuid.UUID) ([]model.RecurringRule, error)
	DueRules(ctx context.Context, today time.Time, userID *uuid.UUID, mode string) ([]model.RecurringRule, error)
	Handled(ctx context.Context, ruleID uuid.UUID, from time.Time) (map[string]bool, error)
	HandleOccurrence(ctx context.Context, userID, ruleID uuid.UUID, due time.Time, e *model.Entry, next time.Time) error
	SetRuleError(ctx context.Context, ruleID uuid.UUID, msg string) error
}

// RecurringService turns repeating rules into entries: reminders wait for a confirm,
// auto rules post themselves (RunAuto), each occurrence exactly once.
type RecurringService struct {
	repo   RecurringRepository
	ledger *LedgerService
	now    func() time.Time
}

func NewRecurringService(repo RecurringRepository, ledger *LedgerService) *RecurringService {
	return &RecurringService{repo: repo, ledger: ledger, now: time.Now}
}

func (s *RecurringService) today() time.Time {
	y, m, d := s.now().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func (s *RecurringService) pattern(ctx context.Context, r model.RecurringRule) (recurring.Pattern, error) {
	p := recurring.Pattern{Frequency: r.Frequency}
	if r.Day != nil {
		p.Day = *r.Day
	}
	if r.Frequency == "payday" {
		day, err := s.repo.PeriodStartDay(ctx, r.UserID)
		if err != nil {
			return p, err
		}
		p.Day = day
	}
	return p, nil
}

// Save creates (id nil) or replaces a rule. The rule is checked by building its entry once,
// so a wrong account pair fails now instead of at its first due date.
func (s *RecurringService) Save(ctx context.Context, userID uuid.UUID, id *uuid.UUID, req model.RecurringRequest) (*model.RecurringRule, error) {
	switch {
	case req.Frequency == "weekly" && (req.Day == nil || *req.Day > 6),
		req.Frequency == "monthly" && (req.Day == nil || *req.Day < 1):
		return nil, ErrNeedsDay
	}
	if _, _, err := s.ledger.buildSimple(ctx, userID, s.entryRequest(model.RecurringRule{Kind: req.Kind,
		FromAccountID: req.FromAccountID, ToAccountID: req.ToAccountID, Amount: req.Amount, Name: req.Name}, req.StartDate, 0)); err != nil {
		return nil, err
	}
	rule := model.RecurringRule{UserID: userID, Name: req.Name, Kind: req.Kind, FromAccountID: req.FromAccountID,
		ToAccountID: req.ToAccountID, Amount: req.Amount, Frequency: req.Frequency, Day: req.Day, Mode: req.Mode, EndDate: req.EndDate}
	if req.Frequency == "daily" || req.Frequency == "payday" {
		rule.Day = nil
	}
	p, err := s.pattern(ctx, rule)
	if err != nil {
		return nil, err
	}
	start, _ := time.Parse(time.DateOnly, req.StartDate)
	rule.NextDate = p.OnOrAfter(start).Format(time.DateOnly)
	if id != nil {
		rule.ID = *id
	}
	if err := s.repo.SaveRule(ctx, &rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

func (s *RecurringService) Delete(ctx context.Context, userID, id uuid.UUID) error {
	return s.repo.DeleteRule(ctx, userID, id)
}

func (s *RecurringService) Rules(ctx context.Context, userID uuid.UUID) ([]model.RecurringRule, error) {
	return s.repo.Rules(ctx, userID)
}

// open lists a rule's unhandled dates up to `until`.
func (s *RecurringService) open(ctx context.Context, r model.RecurringRule, until time.Time) ([]time.Time, error) {
	p, err := s.pattern(ctx, r)
	if err != nil {
		return nil, err
	}
	next, _ := time.Parse(time.DateOnly, r.NextDate)
	if r.EndDate != nil {
		end, _ := time.Parse(time.DateOnly, *r.EndDate)
		until = minTime(until, end)
	}
	handled, err := s.repo.Handled(ctx, r.ID, next)
	if err != nil {
		return nil, err
	}
	var out []time.Time
	for _, d := range p.Between(next, until) {
		if !handled[d.Format(time.DateOnly)] {
			out = append(out, d)
		}
	}
	return out, nil
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

// Due runs the user's auto rules, then returns what still waits for a confirm or skip.
func (s *RecurringService) Due(ctx context.Context, userID uuid.UUID) ([]model.DueOccurrence, error) {
	if err := s.runAuto(ctx, &userID); err != nil {
		return nil, err
	}
	rules, err := s.repo.DueRules(ctx, s.today(), &userID, "")
	if err != nil {
		return nil, err
	}
	out := []model.DueOccurrence{}
	for _, r := range rules {
		dates, err := s.open(ctx, r, s.today())
		if err != nil {
			return nil, err
		}
		for _, d := range dates {
			out = append(out, model.DueOccurrence{Rule: r, DueDate: d.Format(time.DateOnly)})
		}
	}
	return out, nil
}

// RunAuto posts every due occurrence of every auto rule. Safe to run twice or in parallel:
// each occurrence is claimed by its primary key before anything is posted.
func (s *RecurringService) RunAuto(ctx context.Context) error { return s.runAuto(ctx, nil) }

func (s *RecurringService) runAuto(ctx context.Context, userID *uuid.UUID) error {
	rules, err := s.repo.DueRules(ctx, s.today(), userID, "auto")
	if err != nil {
		return err
	}
	for _, r := range rules {
		dates, err := s.open(ctx, r, s.today())
		if err != nil {
			return err
		}
		for _, d := range dates {
			err := s.handle(ctx, r, d, true, 0, d)
			if errors.Is(err, ErrOccurrenceHandled) {
				continue
			}
			if err != nil {
				// The occurrence stays open and shows as a reminder with the reason.
				if err := s.repo.SetRuleError(ctx, r.ID, err.Error()); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

// Confirm posts one open occurrence, optionally with another amount or date.
func (s *RecurringService) Confirm(ctx context.Context, userID, id uuid.UUID, req model.ConfirmRequest) error {
	r, due, err := s.openOccurrence(ctx, userID, id, req.DueDate)
	if err != nil {
		return err
	}
	date := due
	if req.EntryDate != "" {
		date, _ = time.Parse(time.DateOnly, req.EntryDate)
	}
	return s.handle(ctx, r, due, true, req.Amount, date)
}

func (s *RecurringService) Skip(ctx context.Context, userID, id uuid.UUID, req model.SkipRequest) error {
	r, due, err := s.openOccurrence(ctx, userID, id, req.DueDate)
	if err != nil {
		return err
	}
	return s.handle(ctx, r, due, false, 0, due)
}

func (s *RecurringService) openOccurrence(ctx context.Context, userID, id uuid.UUID, dueDate string) (model.RecurringRule, time.Time, error) {
	rules, err := s.repo.Rules(ctx, userID)
	if err != nil {
		return model.RecurringRule{}, time.Time{}, err
	}
	for _, r := range rules {
		if r.ID != id {
			continue
		}
		dates, err := s.open(ctx, r, s.today())
		if err != nil {
			return r, time.Time{}, err
		}
		for _, d := range dates {
			if d.Format(time.DateOnly) == dueDate {
				return r, d, nil
			}
		}
		return r, time.Time{}, ErrNotDue
	}
	return model.RecurringRule{}, time.Time{}, ErrRuleNotFound
}

func (s *RecurringService) entryRequest(r model.RecurringRule, date string, amount int64) model.PostEntryRequest {
	if amount == 0 {
		amount = r.Amount
	}
	return model.PostEntryRequest{Kind: r.Kind, EntryDate: date, Amount: amount, FromAccountID: r.FromAccountID,
		ToAccountID: r.ToAccountID, Description: r.Name}
}

// handle posts (post=true) or skips one occurrence and moves next_date past the handled ones.
func (s *RecurringService) handle(ctx context.Context, r model.RecurringRule, due time.Time, post bool, amount int64, date time.Time) error {
	var e *model.Entry
	if post {
		built, accounts, err := s.ledger.buildSimple(ctx, r.UserID, s.entryRequest(r, date.Format(time.DateOnly), amount))
		if err != nil {
			return err
		}
		if err := s.ledger.prepare(ctx, r.UserID, built, accounts); err != nil {
			return err
		}
		e = built
	}
	next, err := s.nextAfter(ctx, r, due)
	if err != nil {
		return err
	}
	return s.repo.HandleOccurrence(ctx, r.UserID, r.ID, due, e, next)
}

// nextAfter is the first date from next_date on that stays open once `due` is handled.
func (s *RecurringService) nextAfter(ctx context.Context, r model.RecurringRule, due time.Time) (time.Time, error) {
	p, err := s.pattern(ctx, r)
	if err != nil {
		return time.Time{}, err
	}
	next, _ := time.Parse(time.DateOnly, r.NextDate)
	handled, err := s.repo.Handled(ctx, r.ID, next)
	if err != nil {
		return time.Time{}, err
	}
	handled[due.Format(time.DateOnly)] = true
	for d := p.OnOrAfter(next); ; d = p.OnOrAfter(d.AddDate(0, 0, 1)) {
		if !handled[d.Format(time.DateOnly)] {
			return d, nil
		}
	}
}

// StartJob runs RunAuto now and then every interval until ctx ends.
func (s *RecurringService) StartJob(ctx context.Context, every time.Duration) {
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			if err := s.RunAuto(ctx); err != nil {
				log.Printf("recurring job: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}
