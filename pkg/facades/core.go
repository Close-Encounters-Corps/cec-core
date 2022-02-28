package facades

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Close-Encounters-Corps/cec-core/pkg/auth"
	"github.com/Close-Encounters-Corps/cec-core/pkg/auth/tokens"
	"github.com/Close-Encounters-Corps/cec-core/pkg/config"
	"github.com/Close-Encounters-Corps/cec-core/pkg/discord"
	"github.com/Close-Encounters-Corps/cec-core/pkg/items"
	"github.com/Close-Encounters-Corps/cec-core/pkg/tracer"
	"github.com/Close-Encounters-Corps/cec-core/pkg/users"
	"github.com/jackc/pgx/v4/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var CORE_FACADE = "auth_facade"

func NewCoreFacade(
	db *pgxpool.Pool,
	um *users.UserModule,
	dm *discord.DiscordModule,
	tm *tokens.TokenModule,
	cfg *config.Config,
) *CoreFacade {
	return &CoreFacade{
		db:      db,
		users:   um,
		discord: dm,
		tokens:  tm,
		config:  cfg,
		auth: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
}

type CoreFacade struct {
	db      *pgxpool.Pool
	users   *users.UserModule
	discord *discord.DiscordModule
	tokens  *tokens.TokenModule
	config  *config.Config
	auth    *http.Client
}

func (f *CoreFacade) Authenticate(ctx context.Context, kind string, state string) (string, error) {
	ctx, span := tracer.NewSpan(ctx, "core.authenticate", nil)
	defer span.End()
	tx, err := f.db.Begin(ctx)
	span.AddEvent("tx.begin")
	if err != nil {
		return "", err
	}
	defer func() {
		span.AddEvent("tx.rollback")
		tx.Rollback(ctx)
	}()
	authUrl, err := url.Parse(f.config.AuthInternalUrl)
	if err != nil {
		return "", err
	}
	authUrl, err = authUrl.Parse("/api/exchange")
	if err != nil {
		return "", err
	}
	q := authUrl.Query()
	q.Add("secret", f.config.AuthSecret)
	q.Add("state", state)
	q.Add("kind", kind)
	authUrl.RawQuery = q.Encode()
	req, err := http.NewRequest("GET", authUrl.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := f.auth.Do(req)
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		span.AddEvent("response error", trace.WithAttributes(
			attribute.Int("response.code", resp.StatusCode),
			attribute.String("response.body", string(body)),
		))
		if resp.StatusCode == http.StatusBadRequest {
			switch string(body) {
			case "state_not_found":
				return "", fmt.Errorf("state not found")
			}
		}
		return "", fmt.Errorf("cec-auth request failed")
	}
	var oauth auth.OauthToken
	err = json.Unmarshal(body, &oauth)
	if err != nil {
		span.AddEvent("error decoding body", trace.WithAttributes(
			attribute.String("response.body", string(body)),
		))
		return "", err
	}
	var token string
	switch kind {
	case "discord":
		// fetch account using oauth token
		account, err := f.discord.FetchApi(ctx, &oauth)
		if err != nil {
			return "", err
		}
		var usr *items.User
		// lookup current user from context
		usr, found := auth.FromContext(ctx)
		// not found
		if !found {
			span.AddEvent("discord not found")
			// find existing discord account
			usr, err = f.discord.FindUser(ctx, account.Username, tx)
			if err != nil {
				return "", err
			}
			msg := "user found"
			if usr == nil {
				span.AddEvent("create new user")
				// not found? firstly, create user
				usr, err = f.users.NewUser(ctx, tx)
				if err != nil {
					return "", err
				}
				// then create account and associate it with user
				id, err := f.discord.NewAccount(ctx, account, usr.Id, tx)
				if err != nil {
					return "", err
				}
				account.Id = id
				usr.Discord = account
				msg = "user created"
			}
			span.AddEvent(msg, trace.WithAttributes(
				attribute.Int64("user.id", int64(usr.Id)),
				attribute.Int64("principal.id", int64(usr.Principal.Id)),
			))
			// create new token as its not authenticated in system atm
			token, err = f.tokens.NewToken(ctx, usr.Principal, tx)
			if err != nil {
				return "", err
			}
			span.AddEvent("token created")
		}
		err = f.users.Authenticate(ctx, usr.Id, tx)
		if err != nil {
			return "", err
		}
		span.AddEvent("authenticated successfully")
	}
	err = tx.Commit(ctx)
	return token, err
}
