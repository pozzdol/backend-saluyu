package service

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/pozzdol/backend-saluyu/internal/model"
	"github.com/pozzdol/backend-saluyu/internal/repository"
	"github.com/pozzdol/backend-saluyu/internal/token"
)

var (
	ErrGoalNotFound   = repository.ErrGoalNotFound
	ErrInvalidInvite  = repository.ErrInvalidInvite
	ErrAlreadyInGroup = repository.ErrAlreadyInGroup
)

// Goals lists the user's goals with their progress today.
func (s *LedgerService) Goals(ctx context.Context, userID uuid.UUID) ([]model.Goal, error) {
	goals, err := s.repo.Goals(ctx, userID)
	if err != nil {
		return nil, err
	}
	today := time.Now()
	for i := range goals {
		saved, err := s.repo.BalanceAsOf(ctx, userID, goals[i].AccountID, today)
		if err != nil {
			return nil, err
		}
		goals[i].Fill(saved, today)
	}
	return goals, nil
}

// SaveGoal creates (id nil) or replaces a goal. The money is held in an asset account.
func (s *LedgerService) SaveGoal(ctx context.Context, userID uuid.UUID, id *uuid.UUID, req model.GoalRequest) (*model.Goal, error) {
	found, err := s.repo.FindAccounts(ctx, userID, []uuid.UUID{req.AccountID})
	if err != nil {
		return nil, err
	}
	a, ok := found[req.AccountID]
	if !ok {
		return nil, ErrAccountNotFound
	}
	if a.Type != "Asset" || perContact(a.Code) {
		return nil, fmt.Errorf("%w: a goal is saved in a bank, cash, e-wallet or savings account", ErrWrongAccountType)
	}
	g := model.Goal{UserID: userID, Name: req.Name, Target: req.Target, Deadline: req.Deadline, AccountID: req.AccountID}
	if id != nil {
		g.ID = *id
	}
	if err := s.repo.SaveGoal(ctx, &g); err != nil {
		return nil, err
	}
	saved, err := s.repo.BalanceAsOf(ctx, userID, g.AccountID, time.Now())
	if err != nil {
		return nil, err
	}
	g.Fill(saved, time.Now())
	return &g, nil
}

// CreateInviteLink makes a one-time link into a group, valid for 7 days.
func (s *ShareService) CreateInviteLink(ctx context.Context, claims *token.Claims, groupID uuid.UUID) (*model.InviteLink, error) {
	ok, err := s.repo.IsActiveMember(ctx, groupID, claims.UserID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrNotFound
	}
	raw := rand.Text()
	link := &model.InviteLink{Token: raw, ExpiresAt: time.Now().Add(7 * 24 * time.Hour)}
	if err := s.repo.CreateInviteLink(ctx, groupID, claims.UserID, hashToken(raw), link.ExpiresAt); err != nil {
		return nil, err
	}
	return link, nil
}

// AcceptInviteLink joins the group of a link. The link itself proves the invitation,
// so no email has to match.
func (s *ShareService) AcceptInviteLink(ctx context.Context, claims *token.Claims, raw string) (uuid.UUID, error) {
	return s.repo.AcceptInviteLink(ctx, hashToken(raw), claims.UserID)
}

func (s *LedgerService) DeleteGoal(ctx context.Context, userID, id uuid.UUID) error {
	return s.repo.DeleteGoal(ctx, userID, id)
}
