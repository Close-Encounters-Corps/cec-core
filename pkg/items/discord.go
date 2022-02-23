package items

import (
	"time"
)

type DiscordApiUser struct {
	Id            string
	Username      string
	Discriminator string
	Locale        string
	Avatar        string
	Flags         int64
	PublicFlags   int64 `json:"public_flags"`
	MfaEnabled    bool  `json:"mfa_enabled"`
}

type DiscordAccount struct {
	Id          uint64
	UserId      uint64
	Username    string
	ApiResponse DiscordApiUser
	Created     *time.Time
	Updated     *time.Time

	// token info

	AccessToken    string
	TokenType      string
	TokenExpiresIn time.Time
	RefreshToken   string
}
