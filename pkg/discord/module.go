package discord

// TODO https://discord.com/developers/docs/topics/oauth2#authorization-code-grant-refresh-token-exchange-example

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Close-Encounters-Corps/cec-core/pkg/auth"
	"github.com/Close-Encounters-Corps/cec-core/pkg/items"
	"github.com/jackc/pgx/v4"
)

var MODULE_NAME = "discord"

func NewDiscordModule(api ApiClient) *DiscordModule {
	if api == nil {
		api = &http.Client{
			Timeout: 10 * time.Second,
		}
	}
	return &DiscordModule{
		client: api,
	}
}

type ApiClient interface {
	Do(*http.Request) (*http.Response, error)
}

type DiscordModule struct {
	client ApiClient
}

func (m *DiscordModule) Start(ctx context.Context) error {
	return nil
}

// Request user information using oauth token info
func (m *DiscordModule) FetchApi(ctx context.Context, token *auth.OauthToken) (*items.DiscordAccount, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://discord.com/api/users/@me", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("%v %v", token.TokenType, token.AccessToken))
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var disUser items.DiscordApiUser
	err = json.Unmarshal(raw, &disUser)
	if err != nil {
		return nil, err
	}
	username := fmt.Sprintf("%s#%s", disUser.Username, disUser.Discriminator)
	now := time.Now()
	acc := items.DiscordAccount{
		Username:    username,
		ApiResponse: disUser,
		Created:     &now,
		Updated:     &now,
		// token info
		AccessToken:    token.AccessToken,
		TokenType:      token.TokenType,
		TokenExpiresIn: token.Expiry,
		RefreshToken:   token.RefreshToken,
	}
	return &acc, err
}

// Create account with userId and return saved id
func (m *DiscordModule) NewAccount(ctx context.Context, account *items.DiscordAccount, userId uint64, tx pgx.Tx) (uint64, error) {
	var id uint64
	err := tx.QueryRow(ctx, `
	INSERT INTO discord_accounts (
		user_id,
		username,
		api_response,
		created,
		updated,
		access_token,
		token_type,
		token_expires_in,
		refresh_token
	) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	RETURNING id
	`, userId, account.Username, account.ApiResponse,
		account.Created, account.Updated, account.AccessToken,
		account.TokenType, account.TokenExpiresIn, account.RefreshToken,
	).Scan(&id)
	return id, err
}

func (m *DiscordModule) FindUser(ctx context.Context, username string, tx pgx.Tx) (*items.User, error) {
	pr := &items.Principal{}
	usr := &items.User{}
	dis := &items.DiscordAccount{}
	err := tx.QueryRow(ctx, `
	SELECT
		u.id,
		pr.id,
		pr.is_admin,
		pr.created_on,
		pr.last_login,
		pr.state,
		dis.id,
		dis.username,
		dis.created,
		dis.api_response
	) FROM users usr
	JOIN discord_accounts dis ON usr.id = dis.user_id
	JOIN principals pr ON pr.id = u.principal_id
	WHERE dis.username = $1
	`, username).Scan(
		&usr.Id,
		&pr.Id,
		&pr.Admin,
		&pr.CreatedOn,
		&pr.LastLogin,
		&pr.State,
		&dis.Id,
		&dis.Username,
		&dis.Created,
		&dis.Updated,
		&dis.ApiResponse,
	)
	if err != nil {
		return nil, err
	}
	usr.Principal = pr
	usr.Discord = dis
	return usr, nil
}
