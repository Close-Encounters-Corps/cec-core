package facades

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Close-Encounters-Corps/cec-core/pkg/auth"
	"github.com/Close-Encounters-Corps/cec-core/pkg/auth/tokens"
	"github.com/Close-Encounters-Corps/cec-core/pkg/config"
	"github.com/Close-Encounters-Corps/cec-core/pkg/discord"
	"github.com/Close-Encounters-Corps/cec-core/pkg/items"
	"github.com/Close-Encounters-Corps/cec-core/pkg/users"
	"github.com/jackc/pgx/v4/pgxpool"
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
	tx, err := f.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	authUrl, err := url.Parse(f.config.AuthInternalUrl)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest("GET", authUrl.String(), nil)
	if err != nil {
		return "", err
	}
	resp, err := f.auth.Do(req)
	if err != nil {
		return "", err
	}
	var oauth auth.OauthToken
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(body, &oauth)
	if err != nil {
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
			// find existing discord account
			usr, err = f.discord.FindUser(ctx, account.Username, tx)
			if usr == nil {
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
			}
			// create new token as its not authenticated in system atm
			token, err = f.tokens.NewToken(ctx, usr.Principal, tx)
			if err != nil {
				return "", err
			}
		}
		err = f.users.Authenticate(ctx, usr.Id, tx)
		if err != nil {
			return "", err
		}
	}
	return token, tx.Commit(ctx)
}
