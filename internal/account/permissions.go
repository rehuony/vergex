package account

import "context"

// Permissions is what an account's API key may do, as its venue reports it.
type Permissions struct {
	Read         bool `json:"read"`
	Trade        bool `json:"trade"`
	Withdraw     bool `json:"withdraw"`
	IPRestricted bool `json:"ip_restricted"`
}

// PermissionReader is an account that can ask its venue what its key may do. It only reads,
// and the answer is nil where the venue has no way to say.
type PermissionReader interface {
	Permissions(ctx context.Context) (*Permissions, error)
}
