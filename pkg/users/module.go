package users

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Close-Encounters-Corps/cec-core/pkg/items"
	"github.com/Close-Encounters-Corps/cec-core/pkg/principal"
	"github.com/jackc/pgx/v4"
)

var MODULE_NAME = "users"

func NewUserModule(pm *principal.PrincipalModule) *UserModule {
	return &UserModule{
		pm: pm,
	}
}

type UserModule struct {
	pm *principal.PrincipalModule
}

func (m *UserModule) Start(ctx context.Context) error {
	return nil
}

// Create and return new user with principal.
func (m *UserModule) NewUser(ctx context.Context, tx pgx.Tx) (*items.User, error) {
	usr := items.User{}
	princ, err := m.pm.NewPrincipal(ctx, tx)
	err = tx.QueryRow(ctx, `
	INSERT INTO users(principal_id) VALUES ($1)
	RETURNING id
	`, princ.Id).Scan(&usr.Id)
	if err != nil {
		return nil, err
	}
	log.Println("created new user with id", usr.Id)
	return &usr, nil
}

func (m *UserModule) FindOne(ctx context.Context, id uint64, tx pgx.Tx) (*items.User, error) {
	p := &items.Principal{}
	out := &items.User{
		Id:        id,
		Principal: p,
	}
	err := tx.QueryRow(ctx, `
	SELECT p.id, p.admin, p.created_on, p.last_login, p.state 
	FROM users u
	JOIN principals p ON u.principal_id = p.id
	WHERE id = $1
	`, id).Scan(&p.Id, &p.Admin, &p.CreatedOn, &p.LastLogin, &p.State)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (m *UserModule) Authenticate(ctx context.Context, id uint64, tx pgx.Tx) error {
	usr, err := m.FindOne(ctx, id, tx)
	if err != nil {
		return err
	}
	if usr.Principal.State == items.StateBlocked {
		return fmt.Errorf("User %v blocked", id)
	}
	now := time.Now()
	usr.Principal.LastLogin = &now
	return m.pm.Save(ctx, usr.Principal, tx)
}

func (m *UserModule) FindAll(ctx context.Context, ids []uint64, detailed bool, tx pgx.Tx) ([]*items.User, error) {
	out := make([]*items.User, 0)
	rows, err := tx.Query(ctx, `
	SELECT u.id, p.id, p.admin, p.created_on, p.last_login, p.state
	FROM users u
	JOIN principals p ON u.principal_id = p.id
	WHERE usr.id IN $1
	`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		usr := &items.User{}
		p := items.Principal{}
		err = rows.Scan(&usr.Id, &p.Id, &p.Admin, &p.CreatedOn, &p.LastLogin, &p.State)
		if err != nil {
			return nil, err
		}
		usr.Principal = &p
		out = append(out, usr)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
