package facades

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/Close-Encounters-Corps/cec-core/pkg/auth"
	"github.com/Close-Encounters-Corps/cec-core/pkg/discord"
	"github.com/Close-Encounters-Corps/cec-core/pkg/principal"
	"github.com/Close-Encounters-Corps/cec-core/pkg/users"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type Environment struct {
	ctx    context.Context
	cancel context.CancelFunc
	db     *pgxpool.Pool
	t      *testing.T
	tx     pgx.Tx
}

func NewEnvironment(t *testing.T) *Environment {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	db, err := pgxpool.Connect(ctx, "postgres://postgres:postgres@127.0.0.1:5432/core_test")
	if err != nil {
		t.Error(err)
		return nil
	}
	env := &Environment{
		ctx:    ctx,
		cancel: cancel,
		db:     db,
		t:      t,
	}
	if env.Exec(`CREATE TABLE principals (
		id BIGSERIAL NOT NULL PRIMARY KEY,
		is_admin BOOLEAN NOT NULL,
		created_on TIMESTAMP WITH TIME ZONE NOT NULL,
		last_login TIMESTAMP WITH TIME ZONE,
		state VARCHAR(16) NOT NULL
	)`) {
		return nil
	}
	if env.Exec(`CREATE TABLE users (
		id BIGSERIAL NOT NULL PRIMARY KEY,
		principal_id BIGINT NOT NULL REFERENCES principals(id)
	)`) {
		return nil
	}
	if env.Exec(`CREATE TABLE discord_accounts (
		id BIGSERIAL NOT NULL PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		username VARCHAR(64) UNIQUE NOT NULL,
		api_response JSONB NOT NULL,
		created TIMESTAMP WITH TIME ZONE NOT NULL,
		updated TIMESTAMP WITH TIME ZONE NOT NULL,
		access_token VARCHAR NOT NULL,
		token_type VARCHAR(16),
		token_expires_in TIMESTAMP WITH TIME ZONE NOT NULL,
		refresh_token TEXT NOT NULL
	)`) {
		return nil
	}
	if env.Exec(`CREATE TABLE frontier_accounts (
		id BIGSERIAL NOT NULL PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		cmdr VARCHAR(64) UNIQUE NOT NULL,
		capi_response JSONB NOT NULL,
		created TIMESTAMP WITH TIME ZONE NOT NULL,
		updated TIMESTAMP WITH TIME ZONE NOT NULL,
		access_token VARCHAR NOT NULL,
		token_type VARCHAR(16),
		token_expires_in TIMESTAMP WITH TIME ZONE NOT NULL,
		refresh_token TEXT NOT NULL
	);`) {
		return nil
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Error(err)
		return nil
	}
	env.tx = tx
	return env
}

func (env *Environment) Drop() {
	env.tx.Rollback(env.ctx)
	if env.Exec(`DROP TABLE discord_accounts`) {
		return
	}
	if env.Exec(`DROP TABLE frontier_accounts`) {
		return
	}
	if env.Exec(`DROP TABLE users`) {
		return
	}
	if env.Exec(`DROP TABLE principals`) {
		return
	}
	env.cancel()
}

func (env *Environment) Exec(stmt string) bool {
	_, err := env.db.Exec(env.ctx, stmt)
	if err != nil {
		env.t.Error(err)
	}
	return err != nil
}

func TestNewUser(t *testing.T) {
	env := NewEnvironment(t)
	defer env.Drop()
	mod := users.NewUserModule(principal.NewPrincipalModule())
	usr1, err := mod.NewUser(env.ctx, env.tx)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("id: %v\n", usr1.Id)
	usr2, err := mod.NewUser(env.ctx, env.tx)
	if usr1.Id == usr2.Id {
		t.Error("two different users have same id")
		return
	}
}

func TestFindOne(t *testing.T) {
	env := NewEnvironment(t)
	defer env.Drop()
	mod := users.NewUserModule(principal.NewPrincipalModule())
	usr, err := mod.NewUser(env.ctx, env.tx)
	if err != nil {
		t.Error(err)
		return
	}
	usrfo, err := mod.FindOne(env.ctx, usr.Id, env.tx)
	if err != nil {
		t.Error(err)
		return
	}
	if usrfo.Id != usr.Id {
		t.Error("user from findone and newuser has different id")
		return
	}
}

func TestAuthorize(t *testing.T) {
	env := NewEnvironment(t)
	defer env.Drop()
	mod := users.NewUserModule(principal.NewPrincipalModule())
	usr, err := mod.NewUser(env.ctx, env.tx)
	if err != nil {
		t.Error(err)
		return
	}
	err = mod.Authenticate(env.ctx, usr.Id, env.tx)
}

type MockedApi struct {
	Response *http.Response
}

func (m *MockedApi) Do(req *http.Request) (*http.Response, error) {
	return m.Response, nil
}

type MockedResponse struct {
	io.Reader
}

func (r *MockedResponse) Close() error {
	return nil
}

func TestCreateDiscord(t *testing.T) {
	env := NewEnvironment(t)
	defer env.Drop()
	mock := MockedApi{
		Response: &http.Response{
			StatusCode: 200,
			Body: &MockedResponse{
				Reader: strings.NewReader(`{}`),
			},
		},
	}
	// um := users.NewUserModule(principal.NewPrincipalModule())
	mod := discord.NewDiscordModule(&mock)
	// usr, err := um.NewUser(env.ctx, env.tx)
	// if err != nil {
	// 	t.Error(err)
	// 	return
	// }
	account, err := mod.FetchApi(env.ctx, &auth.OauthToken{})
	if err != nil {
		t.Error(err)
		return
	}
	log.Println(account)
}
