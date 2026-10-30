// Package idem makes mutating operations idempotent (BR-07): the response of the first
// successful execution is stored under the caller-supplied key, in the same transaction
// as the business effect, and replayed for every repeat of that key.
package idem

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
)

// Schema is the table each service includes in its migrations.
const Schema = `CREATE TABLE IF NOT EXISTS idempotency_keys (
	key text PRIMARY KEY,
	response bytea NOT NULL,
	created_at timestamptz NOT NULL DEFAULT now()
);`

// Lookup serializes concurrent requests with the same key and reports whether a stored
// response was found (and unmarshalled into out). Must be called inside a transaction.
func Lookup(ctx context.Context, tx pgx.Tx, scope, key string, out proto.Message) (bool, error) {
	if key == "" {
		return false, nil
	}
	full := scope + ":" + key
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, full); err != nil {
		return false, err
	}
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT response FROM idempotency_keys WHERE key=$1`, full).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, proto.Unmarshal(raw, out)
}

// Save stores the response for key. Must be called in the same transaction as Lookup.
func Save(ctx context.Context, tx pgx.Tx, scope, key string, msg proto.Message) error {
	if key == "" {
		return nil
	}
	raw, err := proto.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO idempotency_keys(key, response) VALUES ($1, $2)`, scope+":"+key, raw)
	return err
}
