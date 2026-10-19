package service

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/mail"
	"github.com/pozzdol/backend-saluyu/internal/model"
)

// sendFromHour keeps reminders out of the night: the first run after 07:00 sends them.
const sendFromHour = 7

type NotifyRepository interface {
	Prefs(ctx context.Context, userID uuid.UUID) (model.NotifyPrefs, error)
	SetPrefs(ctx context.Context, userID uuid.UUID, p model.NotifyPrefs) error
	Recipients(ctx context.Context) ([]model.NotifyUser, error)
	Claim(ctx context.Context, userID uuid.UUID, kind, ref string, on time.Time) (bool, error)
	Unclaim(ctx context.Context, userID uuid.UUID, kind, ref string) error
}

// NotifyService emails reminders. Each reminder is claimed in notification_log before it is
// sent, so running the job again (or two instances at once) never sends one twice.
type NotifyService struct {
	repo      NotifyRepository
	reports   *ReportService
	recurring *RecurringService
	credit    *CreditService
	mail      mail.Sender
	appURL    string
}

func NewNotifyService(repo NotifyRepository, reports *ReportService, recurring *RecurringService, credit *CreditService, sender mail.Sender, appURL string) *NotifyService {
	return &NotifyService{repo: repo, reports: reports, recurring: recurring, credit: credit, mail: sender, appURL: appURL}
}

func (s *NotifyService) Prefs(ctx context.Context, userID uuid.UUID) (model.NotifyPrefs, error) {
	return s.repo.Prefs(ctx, userID)
}

func (s *NotifyService) SetPrefs(ctx context.Context, userID uuid.UUID, req model.NotifyPrefsRequest) (model.NotifyPrefs, error) {
	p := model.NotifyPrefs{DueBills: *req.DueBills, Budget: *req.Budget, Weekly: *req.Weekly, Inactive: *req.Inactive}
	return p, s.repo.SetPrefs(ctx, userID, p)
}

// StartJob runs the reminders now and then every interval until ctx ends.
func (s *NotifyService) StartJob(ctx context.Context, every time.Duration) {
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			if err := s.Run(ctx, time.Now()); err != nil {
				log.Printf("notify job: %v", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

// reminder is one line of an email and the key that makes it send once.
type reminder struct{ ref, line string }

// Run evaluates every kind for every verified user as of `now`. One user failing does not stop the rest.
// ponytail: "today" is the server's calendar day; per-user time zones would need a column.
func (s *NotifyService) Run(ctx context.Context, now time.Time) error {
	if now.Hour() < sendFromHour {
		return nil
	}
	users, err := s.repo.Recipients(ctx)
	if err != nil {
		return err
	}
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	for _, u := range users {
		if err := s.runUser(ctx, u, today); err != nil {
			log.Printf("notify %s: %v", u.ID, err)
		}
	}
	return nil
}

func (s *NotifyService) runUser(ctx context.Context, u model.NotifyUser, today time.Time) error {
	lang := "id"
	if u.Locale != nil && *u.Locale == "en" {
		lang = "en"
	}
	kinds := []struct {
		kind string
		on   bool
		eval func(context.Context, uuid.UUID, string, time.Time) ([]reminder, error)
	}{
		{"due_bills", u.Prefs.DueBills, s.dueBills},
		{"budget", u.Prefs.Budget, s.budgetAlerts},
		{"weekly", u.Prefs.Weekly, s.weekly},
		{"inactive", u.Prefs.Inactive, s.inactive},
	}
	for _, k := range kinds {
		if !k.on {
			continue
		}
		items, err := k.eval(ctx, u.ID, lang, today)
		if err != nil {
			return err
		}
		if err := s.send(ctx, u, k.kind, lang, today, items); err != nil {
			return err
		}
	}
	return nil
}

// send claims the reminders, emails the new ones in one message, and gives the claims back if the email fails.
func (s *NotifyService) send(ctx context.Context, u model.NotifyUser, kind, lang string, today time.Time, items []reminder) error {
	var fresh []reminder
	for _, it := range items {
		ok, err := s.repo.Claim(ctx, u.ID, kind, it.ref, today)
		if err != nil {
			return err
		}
		if ok {
			fresh = append(fresh, it)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	lines := make([]string, len(fresh))
	for i, it := range fresh {
		lines[i] = it.line
	}
	t := notifyTexts[kind+"/"+lang]
	greeting, button, footer := "Hi %s,", "Open Saluyu", "You get this because the reminder is on. Turn it off in Settings: "
	if lang == "id" {
		greeting, button, footer = "Halo %s,", "Buka Saluyu", "Kamu menerima ini karena pengingatnya aktif. Matikan di Pengaturan: "
	}
	msg := mail.Digest{Lang: lang, AppURL: s.appURL, Subject: t[0], Title: t[1], Greeting: fmt.Sprintf(greeting, u.Name), Intro: t[2],
		Items: lines, Button: button, URL: s.appURL + "/app/dashboard", Footer: footer + s.appURL + "/account"}.Message()
	msg.To = u.Email
	if err := s.mail.Send(ctx, msg); err != nil {
		for _, it := range fresh {
			if uerr := s.repo.Unclaim(ctx, u.ID, kind, it.ref); uerr != nil {
				log.Printf("notify unclaim: %v", uerr)
			}
		}
		return err
	}
	return nil
}

var notifyTexts = map[string][3]string{
	"due_bills/id": {"Tagihan segera jatuh tempo", "Tagihan segera jatuh tempo", "Ini yang jatuh tempo hari ini atau 3 hari lagi:"},
	"due_bills/en": {"Bills coming due", "Bills coming due", "These are due today or in 3 days:"},
	"budget/id":    {"Anggaranmu hampir habis", "Anggaran perlu perhatian", "Pengeluaran periode ini sudah mencapai batas:"},
	"budget/en":    {"Your budget is running out", "Budgets need attention", "Spending this period has reached the limit:"},
	"weekly/id":    {"Ringkasan mingguanmu", "Ringkasan minggu ini", "Begini 7 hari terakhirmu:"},
	"weekly/en":    {"Your weekly summary", "Your week in short", "Here is how your last 7 days went:"},
	"inactive/id":  {"Sudah 3 hari belum mencatat", "Jangan lupa mencatat", "Tidak ada transaksi tercatat beberapa hari ini:"},
	"inactive/en":  {"3 days without an entry", "Don't forget to record", "No entries were recorded these past days:"},
}

func pick(lang, id, en string) string {
	if lang == "id" {
		return id
	}
	return en
}

// dueIn words how far a due date is; only 0 and 3 days are ever sent.
func dueIn(lang string, days int) string {
	if days == 0 {
		return pick(lang, "hari ini", "today")
	}
	return pick(lang, fmt.Sprintf("%d hari lagi", days), fmt.Sprintf("in %d days", days))
}

// dueBills: recurring rules, the next installment of each loan and the open card bill, due today or in 3 days.
func (s *NotifyService) dueBills(ctx context.Context, userID uuid.UUID, lang string, today time.Time) ([]reminder, error) {
	var out []reminder
	add := func(id string, due time.Time, name string, amount int64) {
		if days := int(due.Sub(today).Hours() / 24); days == 0 || days == 3 {
			out = append(out, reminder{fmt.Sprintf("due:%s:%s:h%d", id, due.Format(time.DateOnly), days),
				fmt.Sprintf("%s: %s (%s)", name, rupiah(amount), dueIn(lang, days))})
		}
	}
	rules, err := s.recurring.Rules(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, r := range rules {
		if r.Kind == "income" {
			continue
		}
		dates, err := s.recurring.open(ctx, r, today.AddDate(0, 0, 3))
		if err != nil {
			return nil, err
		}
		for _, d := range dates {
			add(r.ID.String(), d, r.Name, r.Amount)
		}
	}
	plans, err := s.credit.Plans(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, p := range plans {
		if p.Next != nil {
			add(fmt.Sprintf("%s:%d", p.ID, p.Next.Seq), p.Next.DueDate, p.Account.Name, p.Next.Payment)
		}
	}
	cards, err := s.credit.Cards(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, c := range cards {
		if due, err := time.Parse(time.DateOnly, c.Last.DueDate); err == nil && c.Last.Status == "due" {
			add(c.Account.ID.String(), due, c.Account.Name, c.Last.Remaining)
		}
	}
	return out, nil
}

// budgetAlerts: a category at 80% of its budget, and again at 100%, once per period.
func (s *NotifyService) budgetAlerts(ctx context.Context, userID uuid.UUID, lang string, today time.Time) ([]reminder, error) {
	ins, err := s.reports.Insights(ctx, userID, today)
	if err != nil {
		return nil, err
	}
	var out []reminder
	for _, a := range ins.BudgetAlerts {
		level := "80"
		if a.Level == "over" {
			level = "100"
		}
		out = append(out, reminder{fmt.Sprintf("budget%s:%s:%s", level, a.AccountID, ins.Period.Start),
			fmt.Sprintf("%s: %s / %s (%d%%)", a.Name, rupiah(a.Actual), rupiah(a.Budget), a.Actual*100/a.Budget)})
	}
	return out, nil
}

// weekly: on Mondays, the last 7 days of spending, the top 3 categories and today's safe-per-day.
func (s *NotifyService) weekly(ctx context.Context, userID uuid.UUID, lang string, today time.Time) ([]reminder, error) {
	if today.Weekday() != time.Monday {
		return nil, nil
	}
	moves, err := s.reports.repo.Movements(ctx, userID, today.AddDate(0, 0, -7), today, false)
	if err != nil {
		return nil, err
	}
	rows, total := amounts(moves, ofType("Expense"))
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Amount > rows[j].Amount })
	ins, err := s.reports.Insights(ctx, userID, today)
	if err != nil {
		return nil, err
	}
	lines := []string{
		pick(lang, "Pengeluaran 7 hari terakhir: ", "Spent in the last 7 days: ") + rupiah(total),
		pick(lang, "Sisa aman per hari: ", "Safe to spend per day: ") + rupiah(ins.SafePerDay.Amount),
	}
	var top []string
	for _, r := range rows[:min(3, len(rows))] {
		top = append(top, fmt.Sprintf("%s %s", r.Name, rupiah(r.Amount)))
	}
	if len(top) > 0 {
		lines = append(lines, pick(lang, "Terbesar: ", "Top categories: ")+strings.Join(top, ", "))
	}
	out := make([]reminder, len(lines))
	for i, l := range lines {
		out[i] = reminder{"weekly:" + today.Format(time.DateOnly) + ":" + strconv.Itoa(i), l}
	}
	return out, nil
}

// inactive: the last entry is 3 or more days old. Users who never recorded anything are left alone.
func (s *NotifyService) inactive(ctx context.Context, userID uuid.UUID, lang string, today time.Time) ([]reminder, error) {
	days, err := s.reports.repo.EntryDays(ctx, userID, time.Time{})
	if err != nil || len(days) == 0 || today.Sub(days[0]) < 3*24*time.Hour {
		return nil, err
	}
	last := days[0].Format(time.DateOnly)
	return []reminder{{"inactive:" + last, pick(lang, "Transaksi terakhirmu tercatat tanggal ", "Your last entry was on ") + last}}, nil
}
