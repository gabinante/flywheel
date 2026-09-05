package auth

// Identity describes the operator identity provisioned at startup. The field
// names are inherited from the GitHub OAuth era so the provisioner and user
// store stay unchanged; Flywheel is now single-operator and local-first.
type Identity struct {
	ID        int64  `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
}

// LocalOperator is the fixed identity reused across local server restarts.
func LocalOperator() *Identity {
	return &Identity{ID: -1, Login: "operator", Name: "Local Operator", Email: "operator@localhost"}
}
