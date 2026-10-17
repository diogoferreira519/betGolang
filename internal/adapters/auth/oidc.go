package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

type Principal struct {
	Subject, ClientID, ProviderID string
	Roles map[string]bool
	Internal bool
}

type Validator struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	internalClientID string
}

func NewValidator(ctx context.Context, issuerURL, audience, internalClientID string) (*Validator, error) {
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidc provider: %w", err)
	}
	verifier := provider.Verifier(&oidc.Config{ClientID: audience, SkipClientIDCheck: audience == "" || audience == "account", SupportedSigningAlgs: []string{"RS256"}})
	return &Validator{provider: provider, verifier: verifier, internalClientID: internalClientID}, nil
}

func (v *Validator) Authenticate(ctx context.Context, r *http.Request) (*Principal, error) {
	h := r.Header.Get("Authorization")
	if h == "" || !strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return nil, fmt.Errorf("missing bearer token")
	}
	raw := strings.TrimSpace(h[7:])
	tok, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	var claims struct {
		Sub string `json:"sub"`
		Azp string `json:"azp"`
		PreferredUsername string `json:"preferred_username"`
		RealmAccess struct{ Roles []string `json:"roles"` } `json:"realm_access"`
	}
	if err := tok.Claims(&claims); err != nil {
		return nil, err
	}
	clientID := claims.Azp
	if clientID == "" {
		clientID = claims.PreferredUsername
	}
	roles := map[string]bool{}
	for _, role := range claims.RealmAccess.Roles {
		roles[role] = true
	}
	return &Principal{Subject: claims.Sub, ClientID: clientID, ProviderID: clientID, Roles: roles, Internal: clientID == v.internalClientID || roles["wallet:admin"]}, nil
}

type contextKey int
const principalKey contextKey = 1

func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}
func FromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(principalKey).(*Principal)
	return p, ok
}
func (v *Validator) Ready(ctx context.Context) error {
	_, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return nil
}
