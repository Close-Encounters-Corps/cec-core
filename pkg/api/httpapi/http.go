package httpapi

import (
	"github.com/Close-Encounters-Corps/cec-core/pkg/items"
)

type Error struct {

	// message
	Message string `json:"message,omitempty"`

	// request id
	RequestID string `json:"request_id,omitempty"`
}

type AuthPhaseResult struct {

	// next url
	NextURL string `json:"next_url,omitempty"`

	// phase
	Phase int32 `json:"phase,omitempty"`

	// token
	Token string `json:"token,omitempty"`

	// user
	User *items.User `json:"user,omitempty"`
}
