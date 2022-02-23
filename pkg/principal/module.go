package principal

import (
	"context"
	"time"

	"github.com/Close-Encounters-Corps/cec-core/pkg/items"
	"github.com/jackc/pgx/v4"
)

var MODULE_NAME = "principal"

func NewPrincipalModule() *PrincipalModule {
	return &PrincipalModule{}
}

type PrincipalModule struct {
}

func (m *PrincipalModule) Start(ctx context.Context) error {
	return nil
}

func (m *PrincipalModule) NewPrincipal(ctx context.Context, tx pgx.Tx) (*items.Principal, error) {
	now := time.Now()
	p := items.Principal{
		CreatedOn: &now,
		State:     items.StatePending,
	}
	err := tx.QueryRow(ctx, `
	INSERT INTO principals(
		is_admin, 
		created_on, 
		last_login, 
		state
	) VALUES ($1, $2, $3, $4)
	RETURNING id
	`, p.Admin, p.CreatedOn, p.LastLogin, p.State).
		Scan(&p.Id)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (m *PrincipalModule) Save(ctx context.Context, p *items.Principal, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
	UPDATE principals SET 
		is_admin = $2,
		last_login = $3,
		state = $4
	WHERE id = $1
	`, p.Id, p.Admin, p.CreatedOn, p.State)
	return err
}
