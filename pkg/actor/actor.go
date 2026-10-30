// Package actor carries the authenticated caller (user or system) through request contexts.
package actor

import (
	"context"
	"slices"
)

type Actor struct {
	OrgID         string
	UserID        string
	Email         string
	Roles         []string
	DepotIDs      []string
	CorrelationID string
}

type ctxKey struct{}

func With(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, ctxKey{}, a)
}

func From(ctx context.Context) Actor {
	a, _ := ctx.Value(ctxKey{}).(Actor)
	return a
}

// Name is the identity recorded in ledgers and audit trails.
func (a Actor) Name() string {
	if a.Email != "" {
		return a.Email
	}
	if a.UserID != "" {
		return a.UserID
	}
	return "system"
}

func (a Actor) HasRole(roles ...string) bool {
	for _, r := range roles {
		if slices.Contains(a.Roles, r) {
			return true
		}
	}
	return false
}

// CanAccessDepot reports whether the actor may operate on the depot.
// An empty depot list means access to every depot in the organization.
func (a Actor) CanAccessDepot(id string) bool {
	return len(a.DepotIDs) == 0 || id == "" || slices.Contains(a.DepotIDs, id)
}
