package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
	"github.com/pozzdol/backend-saluyu/internal/statement"
)

var (
	ErrUnreadableFile = statement.ErrUnreadable
	ErrNoColumns      = errors.New("could not find the date and amount columns; set them by hand")
	ErrRuleMissing    = repository.ErrRuleMissing
	ErrImportAccount  = errors.New("statements import into a bank, cash, e-wallet or card account")
)

type ImportRepository interface {
	FindAccounts(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]model.Account, error)
	ListAccounts(ctx context.Context, userID uuid.UUID) ([]model.Account, error)
	CategoryRules(ctx context.Context, userID uuid.UUID) ([]model.CategoryRule, error)
	CreateCategoryRule(ctx context.Context, userID uuid.UUID, c *model.CategoryRule) error
	DeleteCategoryRule(ctx context.Context, userID, id uuid.UUID) error
	ImportProfile(ctx context.Context, accountID uuid.UUID) (*statement.Mapping, error)
	ImportedHashes(ctx context.Context, accountID uuid.UUID, hashes []string) (map[string]uuid.UUID, error)
	MatchCandidates(ctx context.Context, userID, accountID uuid.UUID, from, to time.Time) ([]model.MatchCandidate, error)
	CommitImport(ctx context.Context, userID, accountID uuid.UUID, rows []model.ImportedRow, mapping statement.Mapping) (posted, matched, skipped int, err error)
}

type ImportService struct {
	repo   ImportRepository
	ledger *LedgerService
	now    func() time.Time
}

func NewImportService(repo ImportRepository, ledger *LedgerService) *ImportService {
	return &ImportService{repo: repo, ledger: ledger, now: time.Now}
}

func (s *ImportService) Rules(ctx context.Context, userID uuid.UUID) ([]model.CategoryRule, error) {
	return s.repo.CategoryRules(ctx, userID)
}

func (s *ImportService) CreateRule(ctx context.Context, userID uuid.UUID, req model.CategoryRuleRequest) (*model.CategoryRule, error) {
	found, err := s.repo.FindAccounts(ctx, userID, []uuid.UUID{req.AccountID})
	if err != nil {
		return nil, err
	}
	if _, ok := found[req.AccountID]; !ok {
		return nil, ErrAccountNotFound
	}
	c := model.CategoryRule{Keyword: req.Keyword, MinAmount: req.MinAmount, MaxAmount: req.MaxAmount, AccountID: req.AccountID}
	if err := s.repo.CreateCategoryRule(ctx, userID, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *ImportService) DeleteRule(ctx context.Context, userID, id uuid.UUID) error {
	return s.repo.DeleteCategoryRule(ctx, userID, id)
}

func (s *ImportService) account(ctx context.Context, userID, id uuid.UUID) (model.Account, error) {
	found, err := s.repo.FindAccounts(ctx, userID, []uuid.UUID{id})
	if err != nil {
		return model.Account{}, err
	}
	a, ok := found[id]
	if !ok {
		return a, ErrAccountNotFound
	}
	if !(a.Type == "Asset" || a.IsCard()) || perContact(a.Code) {
		return a, ErrImportAccount
	}
	return a, nil
}

// rowHash identifies a statement row across imports. n counts identical rows before it in
// the same file, so two equal coffees on one day stay two rows.
func rowHash(r statement.Row, n int) string {
	desc := strings.ToLower(strings.Join(strings.Fields(r.Description), " "))
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s|%d", r.Date, r.Amount, desc, n)))
	return hex.EncodeToString(sum[:])
}

// Preview reads the file and labels every row: new, duplicate (imported before) or matched
// (an entry recorded by hand with the same amount within 3 days), with a suggested category.
func (s *ImportService) Preview(ctx context.Context, userID, accountID uuid.UUID, data []byte, mapping *statement.Mapping) (*model.ImportPreview, error) {
	if _, err := s.account(ctx, userID, accountID); err != nil {
		return nil, err
	}
	cells, err := statement.Read(data)
	if err != nil {
		return nil, err
	}
	if mapping == nil {
		saved, err := s.repo.ImportProfile(ctx, accountID)
		if err != nil {
			return nil, err
		}
		if saved != nil && saved.Fits(cells) {
			mapping = saved
		}
	}
	if mapping == nil {
		m, err := statement.Detect(cells)
		if err != nil {
			return nil, fmt.Errorf("%w (%v)", ErrNoColumns, err)
		}
		mapping = &m
	}
	parsed, lineErrs := statement.Parse(cells, *mapping, s.now())
	if mapping.HeaderRow >= 0 && mapping.HeaderRow < len(cells) {
		mapping.Headers = cells[mapping.HeaderRow] // a hand-made mapping is remembered for this layout
	}
	out := &model.ImportPreview{Mapping: *mapping, Rows: make([]model.ImportRow, len(parsed)), Errors: lineErrs, Headers: []string{}}
	if mapping.HeaderRow >= 0 && mapping.HeaderRow < len(cells) {
		out.Headers = cells[mapping.HeaderRow]
	}
	if out.Errors == nil {
		out.Errors = []statement.LineError{}
	}
	if len(parsed) == 0 {
		return out, nil
	}

	seen := map[string]int{}
	hashes := make([]string, len(parsed))
	first, last := parsed[0].Date, parsed[0].Date
	for i, r := range parsed {
		key := fmt.Sprintf("%s|%d|%s", r.Date, r.Amount, strings.ToLower(strings.Join(strings.Fields(r.Description), " ")))
		hashes[i] = rowHash(r, seen[key])
		seen[key]++
		first, last = min(first, r.Date), max(last, r.Date)
	}
	imported, err := s.repo.ImportedHashes(ctx, accountID, hashes)
	if err != nil {
		return nil, err
	}
	from, _ := time.Parse(time.DateOnly, first)
	to, _ := time.Parse(time.DateOnly, last)
	candidates, err := s.repo.MatchCandidates(ctx, userID, accountID, from.AddDate(0, 0, -3), to.AddDate(0, 0, 3))
	if err != nil {
		return nil, err
	}
	rules, err := s.repo.CategoryRules(ctx, userID)
	if err != nil {
		return nil, err
	}
	list, err := s.repo.ListAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	accounts := map[uuid.UUID]model.Account{}
	for _, a := range list {
		accounts[a.ID] = a
	}
	used := map[uuid.UUID]bool{}
	for i, r := range parsed {
		row := model.ImportRow{Row: r, Hash: hashes[i], Status: "new"}
		if id, ok := imported[hashes[i]]; ok {
			row.Status, row.EntryID = "duplicate", &id
		} else if c := matchCandidate(candidates, used, r); c != nil {
			used[c.EntryID] = true
			row.Status, row.EntryID, row.EntryNo = "matched", &c.EntryID, &c.EntryNo
		} else {
			for _, rule := range rules {
				if rule.Matches(r.Description, r.Amount) && fitsDirection(accounts[rule.AccountID], r.Amount) {
					id := rule.AccountID
					row.SuggestedAccountID = &id
					break
				}
			}
		}
		out.Rows[i] = row
	}
	return out, nil
}

// fitsDirection: money out goes to an expense, money in comes from a revenue; another
// money account fits both ways (a transfer).
func fitsDirection(a model.Account, amount int64) bool {
	if a.Type == "Asset" || a.Type == "Liability" {
		return !perContact(a.Code)
	}
	if amount < 0 {
		return a.Type == "Expense"
	}
	return a.Type == "Revenue"
}

// matchCandidate finds an unused entry with the same signed amount within 3 days, closest first.
func matchCandidate(cs []model.MatchCandidate, used map[uuid.UUID]bool, r statement.Row) *model.MatchCandidate {
	date, _ := time.Parse(time.DateOnly, r.Date)
	var best *model.MatchCandidate
	bestGap := 4
	for i, c := range cs {
		if used[c.EntryID] || c.Amount != r.Amount {
			continue
		}
		d, _ := time.Parse(time.DateOnly, c.Date)
		gap := int(d.Sub(date).Hours() / 24)
		if gap < 0 {
			gap = -gap
		}
		if gap < bestGap {
			best, bestGap = &cs[i], gap
		}
	}
	return best
}

// Commit posts the new rows and links the matched ones. Money out goes to an expense (or,
// for a bank or card, as a transfer); money in comes from a revenue (or another account).
func (s *ImportService) Commit(ctx context.Context, userID uuid.UUID, req model.ImportCommitRequest) (*model.ImportResult, error) {
	acct, err := s.account(ctx, userID, req.AccountID)
	if err != nil {
		return nil, err
	}
	list, err := s.repo.ListAccounts(ctx, userID)
	if err != nil {
		return nil, err
	}
	byCode := map[string]uuid.UUID{}
	for _, a := range list {
		byCode[a.Code] = a.ID
	}
	all := map[uuid.UUID]model.Account{}
	for _, a := range list {
		all[a.ID] = a
	}

	rows := make([]model.ImportedRow, 0, len(req.Rows))
	for _, r := range req.Rows {
		hash, _ := hex.DecodeString(r.Hash) // format checked by binding
		if r.MatchEntryID != nil {
			rows = append(rows, model.ImportedRow{Hash: hash, EntryID: r.MatchEntryID})
			continue
		}
		cat := byCode["5199"]
		if r.Amount > 0 {
			cat = byCode["4199"]
		}
		if r.AccountID != nil {
			cat = *r.AccountID
		}
		other, ok := all[cat]
		if !ok {
			return nil, fmt.Errorf("%w (%s)", ErrAccountNotFound, r.Date)
		}
		e, err := s.rowEntry(ctx, userID, acct, other, r)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", r.Date, r.Description, err)
		}
		rows = append(rows, model.ImportedRow{Hash: hash, Entry: e})
	}

	posted, matched, skipped, err := s.repo.CommitImport(ctx, userID, req.AccountID, rows, req.Mapping)
	if err != nil {
		return nil, err
	}
	for _, rr := range req.Remember {
		if _, err := s.CreateRule(ctx, userID, rr); err != nil {
			return nil, err
		}
	}
	return &model.ImportResult{Posted: posted, Matched: matched, Skipped: skipped}, nil
}

func (s *ImportService) rowEntry(ctx context.Context, userID uuid.UUID, acct, other model.Account, r model.ImportCommitRow) (*model.Entry, error) {
	req := model.PostEntryRequest{EntryDate: r.Date, Description: truncate(r.Description, 200)}
	if r.Amount < 0 {
		req.Amount, req.FromAccountID, req.ToAccountID = -r.Amount, acct.ID, other.ID
		req.Kind = "expense"
		if other.Type != "Expense" {
			req.Kind = "transfer"
		}
	} else {
		req.Amount, req.FromAccountID, req.ToAccountID = r.Amount, other.ID, acct.ID
		req.Kind = "income"
		if other.Type != "Revenue" {
			req.Kind = "transfer"
		}
	}
	e, accounts, err := s.ledger.buildSimple(ctx, userID, req)
	if err != nil {
		return nil, err
	}
	if err := s.ledger.prepare(ctx, userID, e, accounts); err != nil {
		return nil, err
	}
	return e, nil
}
