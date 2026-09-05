package auth

import (
	"fmt"

	"github.com/golang-jwt/jwt/v5"
)

// Claims describes legacy programmatic MCP bearer credentials. The local UI
// does not issue, store, or require JWTs.
type Claims struct {
	jwt.RegisteredClaims
	AgentID string `json:"agent_id"`
}

// VerifyJWT parses and validates the token, returns the agent ID.
func VerifyJWT(secret string, tokenString string) (agentID string, err error) {
	if secret == "" {
		return "", fmt.Errorf("jwt: secret required")
	}
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return "", err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return "", fmt.Errorf("invalid token")
	}
	return claims.AgentID, nil
}
