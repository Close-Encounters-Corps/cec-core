package auth

import (
	"context"

	"github.com/Close-Encounters-Corps/cec-core/pkg/items"
)

type key int

var userKey key

func NewContext(ctx context.Context, user *items.User) context.Context {
	return context.WithValue(ctx, userKey, user)
}

func FromContext(ctx context.Context) (*items.User, bool) {
	u, ok := ctx.Value(userKey).(*items.User)
	return u, ok
}