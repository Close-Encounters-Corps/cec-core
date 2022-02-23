package items

type User struct {
	Id          uint64
	Principal   *Principal
	Discord     *DiscordAccount
}

var (
	StatePending  = "pending"
	StateApproved = "approved"
	StateBlocked  = "blocked"
)
