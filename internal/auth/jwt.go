package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	Issuer  = "flywheel"
	Subject = "agent_id" // JWT sub = agent ID for API/MCP
)

// Claims for our JWT (sub = agent_id so one token works for both human and agent identity).
type Claims struct {
	jwt.RegisteredClaims
	AgentID string `json:"agent_id"`
}

// IssueJWT signs a JWT for the given agent ID. Expiry from now + duration.
func IssueJWT(secret string, agentID string, expiry time.Duration) (string, error) {
	if secret == "" {
		return "", fmt.Errorf("jwt: secret required")
	}
	now := time.Now().UTC()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   agentID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
		},
		AgentID: agentID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ExpiryFromSeconds returns a duration for JWT expiry. If sec <= 0, returns 7 days.
func ExpiryFromSeconds(sec int) time.Duration {
	if sec <= 0 {
		return 7 * 24 * time.Hour
	}
	return time.Duration(sec) * time.Second
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
