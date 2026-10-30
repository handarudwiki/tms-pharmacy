// Package token issues and verifies the JWT access tokens used by the API gateway.
package token

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/handarudwiki/pharmacy-tms/pkg/actor"
	"github.com/handarudwiki/pharmacy-tms/pkg/env"
)

const TTL = 12 * time.Hour

type Claims struct {
	OrgID    string   `json:"org"`
	Email    string   `json:"email"`
	Roles    []string `json:"roles"`
	DepotIDs []string `json:"depots,omitempty"`
	jwt.RegisteredClaims
}

func secret() []byte { return []byte(env.Get("TMS_JWT_SECRET", "dev-tms-jwt-secret-change-me")) }

func Issue(a actor.Actor) (string, time.Time, error) {
	exp := time.Now().Add(TTL)
	claims := Claims{
		OrgID: a.OrgID, Email: a.Email, Roles: a.Roles, DepotIDs: a.DepotIDs,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   a.UserID,
			Issuer:    "tms-identity",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret())
	return s, exp, err
}

func Verify(raw string) (actor.Actor, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(raw, &c, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected signing method")
		}
		return secret(), nil
	}, jwt.WithIssuer("tms-identity"))
	if err != nil {
		return actor.Actor{}, err
	}
	return actor.Actor{OrgID: c.OrgID, UserID: c.Subject, Email: c.Email, Roles: c.Roles, DepotIDs: c.DepotIDs}, nil
}
