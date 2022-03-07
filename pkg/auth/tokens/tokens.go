package tokens

import (
	"context"
	"crypto/rand"
	"encoding/base64"

	"github.com/Close-Encounters-Corps/cec-core/pkg/api"
	"github.com/Close-Encounters-Corps/cec-core/pkg/items"
	"github.com/Close-Encounters-Corps/cec-core/pkg/tracer"
	"github.com/jackc/pgx/v4"
)

var MODULE_NAME = "tokens"

const TOKEN_SIZE = 32

func NewTokenModule() *TokenModule {
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
	token := base64.RawStdEncoding.EncodeToString(b)
	_, err := tx.Exec(ctx, `
	INSERT INTO access_tokens (
		principal_id, token, expires
	) VALUES ($1, $2, $3)
	`, p.Id, token, nil)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (m *TokenModule) FindPrincipalID(ctx context.Context, db api.DbConn, token string) (uint64, error) {
	ctx, span := tracer.NewSpan(ctx, "tokens.find_principal_id", nil)
	defer span.End()
	var id uint64
	err := db.QueryRow(ctx, `
		SELECT principal_id FROM access_tokens
		WHERE token = $1 AND ((expires IS NOT NULL AND expires > now()) OR expires IS NULL)
	`, token).Scan(&id)
	if err != nil {
		tracer.AddSpanError(span, err)
		tracer.FailSpan(span, "query error")
		return 0, err
	}
	return id, nil
}
