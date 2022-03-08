package items

type User struct {
	Id        uint64          `json:"id"`
	Principal *Principal      `json:"principal,omitempty"`
	Discord   *DiscordAccount `json:"discord,omitempty"`
}

var (
	StatePending  = "pending"
	StateApproved = "approved"
	StateBlocked  = "blocked"
)
