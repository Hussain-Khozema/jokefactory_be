package usecase_test

import (
	"context"
	"testing"

	"jokefactory/src/core/domain"
	"jokefactory/src/core/usecase"
)

func joinMany(t *testing.T, session *usecase.SessionService, names []string) []*domain.User {
	t.Helper()
	ctx := context.Background()
	users := make([]*domain.User, 0, len(names))
	for _, name := range names {
		res, err := session.Join(ctx, name)
		if err != nil {
			t.Fatalf("join %s: %v", name, err)
		}
		users = append(users, res.User)
	}
	return users
}

func findJM(t *testing.T, session *usecase.SessionService, users []*domain.User) *domain.User {
	t.Helper()
	ctx := context.Background()
	for _, u := range users {
		me, err := session.Me(ctx, u.ID)
		if err != nil {
			t.Fatalf("me: %v", err)
		}
		if me.User.Role != nil && *me.User.Role == domain.RoleJM {
			return me.User
		}
	}
	t.Fatal("expected a JM with team")
	return nil
}

func findMarketing(t *testing.T, session *usecase.SessionService, users []*domain.User) *domain.User {
	t.Helper()
	ctx := context.Background()
	for _, u := range users {
		me, err := session.Me(ctx, u.ID)
		if err != nil {
			t.Fatalf("me: %v", err)
		}
		if me.User.Role != nil && *me.User.Role == domain.RoleMarketing {
			return me.User
		}
	}
	t.Fatal("expected a MARKETING user")
	return nil
}

func findMarketingOnTeam(t *testing.T, session *usecase.SessionService, users []*domain.User, teamID int64) *domain.User {
	t.Helper()
	ctx := context.Background()
	for _, u := range users {
		me, err := session.Me(ctx, u.ID)
		if err != nil {
			t.Fatalf("me: %v", err)
		}
		if me.User.Role != nil && *me.User.Role == domain.RoleMarketing &&
			me.User.TeamID != nil && *me.User.TeamID == teamID {
			return me.User
		}
	}
	t.Fatal("expected MARKETING on team")
	return nil
}

func findOtherTeamMarketing(t *testing.T, session *usecase.SessionService, users []*domain.User, excludeTeam int64) *domain.User {
	t.Helper()
	ctx := context.Background()
	for _, u := range users {
		me, err := session.Me(ctx, u.ID)
		if err != nil {
			t.Fatalf("me: %v", err)
		}
		if me.User.Role != nil && *me.User.Role == domain.RoleMarketing &&
			me.User.TeamID != nil && *me.User.TeamID != excludeTeam {
			return me.User
		}
	}
	t.Fatal("expected MARKETING on another team")
	return nil
}
