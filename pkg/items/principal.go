package items

import "time"

type Principal struct {
	Id        uint64
	Admin     bool
	CreatedOn *time.Time
	LastLogin *time.Time
	State     string
}
