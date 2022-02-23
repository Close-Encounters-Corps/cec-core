package tokens

import (
	"context"
	"crypto/rand"

	"github.com/Close-Encounters-Corps/cec-core/pkg/items"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

var MODULE_NAME = "tokens"

const TOKEN_SIZE = 32

func NewTokenModule(db pgxpool.Pool) *TokenModule {
	return &TokenModule{}
}

type TokenModule struct {

}

func (m *TokenModule) Start(ctx context.Context) error {
	return nil
}

func (m *TokenModule) NewToken(ctx context.Context, p *items.Principal, tx pgx.Tx) (string, error) {
	b := make([]byte, TOKEN_SIZE)
	rand.Read(b)
	token := string(b)
	_, err := tx.Exec(ctx, `
	INSERT INTO access_tokens (
		principal_id, token, expires
	) VALUES ($1, $2, $3)
	`, p.Id, token, nil)
	if err != nil {
		return "", err
	}
	err = tx.Commit(ctx)
	if err != nil {
		return "", err
	}
	return token, nil
}
