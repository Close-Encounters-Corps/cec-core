package items

import "time"

type Principal struct {
	Id        uint64     `json:"id"`
	Admin     bool       `json:"admin"`
	CreatedOn *time.Time `json:"created_on"`
	LastLogin *time.Time `json:"last_login"`
	State     string     `json:"state"`
}
