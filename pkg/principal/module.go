package principal

import (
	"context"
	"time"

	"github.com/Close-Encounters-Corps/cec-core/pkg/api"
	"github.com/Close-Encounters-Corps/cec-core/pkg/items"
	"github.com/Close-Encounters-Corps/cec-core/pkg/tracer"
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

func (m *PrincipalModule) Save(ctx context.Context, p *items.Principal, tx api.DbConn) error {
	_, err := tx.Exec(ctx, `
	UPDATE principals SET 
		is_admin = $2,
		last_login = $3,
		state = $4
	WHERE id = $1
	`, p.Id, p.Admin, p.CreatedOn, p.State)
	return err
}

func (m *PrincipalModule) FindOne(ctx context.Context, tx pgx.Tx, id uint64) (*items.Principal, error) {
	ctx, span := tracer.NewSpan(ctx, "principals.findone", nil)
	defer span.End()
	p := items.Principal{Id: id}
	err := tx.QueryRow(ctx, `
	SELECT (
		is_admin,
		created_on,
		last_login,
		state
	) FROM principals WHERE id = $1
	`, id).Scan(&p.Admin, &p.CreatedOn, &p.LastLogin, &p.State)
	if err != nil {
		tracer.AddSpanError(span, err)
		tracer.FailSpan(span, "query error")
		return nil, err
	}
	return &p, err
}