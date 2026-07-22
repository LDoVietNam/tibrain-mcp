package security

import (
	"strings"

	"github.com/ti/router/tibrain/internal/config"
)

// Category classifies a tool's safety posture for profile enforcement.
type Category string

const (
	CatRead     Category = "read"      // read/list/search/status
	CatWrite    Category = "write"     // controlled mutation, non-destructive
	CatDestruct Category = "destructive" // delete / stop / restart / privileged
)

// Guard enforces the active permission profile against tool calls.
// It fails closed: any unrecognized operator/action/resource is denied.
type Guard struct {
	profile       config.Profile
	adminIDs      map[string]bool
	trustedArmed  bool
}

// NewGuard builds a guard for the active profile.
func NewGuard(cfg *config.Config) *Guard {
	admins := make(map[string]bool, len(cfg.Permissions.TrustedFull.AdminIdentities))
	for _, id := range cfg.Permissions.TrustedFull.AdminIdentities {
		admins[strings.ToLower(strings.TrimSpace(id))] = true
	}
	return &Guard{
		profile:      cfg.Permissions.ActiveProfile,
		adminIDs:     admins,
		trustedArmed: cfg.TrustedFullArmed(),
	}
}

// Allow reports whether an identity may invoke a tool of the given category.
// identity is the resolved gateway identity (tok:<hash> or ip:<host>).
func (g *Guard) Allow(identity string, cat Category) error {
	// Fail closed on any category that is not one of the defined safety tiers.
	if cat != CatRead && cat != CatWrite && cat != CatDestruct {
		return &PermError{Status: 403, Msg: "unknown tool category; failing closed"}
	}
	switch g.profile {
	case config.ProfileReadOnly:
		if cat != CatRead {
			return &PermError{Status: 403, Msg: "read_only profile forbids non-read action"}
		}
		return nil
	case config.ProfileOperator:
		if cat == CatDestruct {
			return &PermError{Status: 403, Msg: "operator profile forbids destructive action"}
		}
		return nil
	case config.ProfileTrustedFull:
		if !g.trustedArmed {
			return &PermError{Status: 403, Msg: "trusted_full not armed; refusing privileged action"}
		}
		// Only admin identities may use trusted_full.
		if !strings.HasPrefix(identity, "tok:") {
			return &PermError{Status: 403, Msg: "trusted_full requires authenticated admin identity"}
		}
		// admin allowlist is enforced at config validation; here we additionally
		// require the identity to map to a known admin hash when provided.
		if len(g.adminIDs) > 0 && !g.knownAdmin(identity) {
			return &PermError{Status: 403, Msg: "identity not in trusted_full admin allowlist"}
		}
		return nil
	default:
		return &PermError{Status: 403, Msg: "unknown profile; failing closed"}
	}
}

// RegisterAdminHash records an admin identity hash so trusted_full can scope to
// specific authenticated admins (set from config admin_identities at startup).
func (g *Guard) RegisterAdminHash(hash string) {
	g.adminIDs[strings.ToLower(hash)] = true
}

func (g *Guard) knownAdmin(identity string) bool {
	if len(g.adminIDs) == 0 {
		return true
	}
	return g.adminIDs[strings.ToLower(identity)]
}

// PermError carries an HTTP-equivalent status for authorization failures.
type PermError struct {
	Status int
	Msg    string
}

func (e *PermError) Error() string { return e.Msg }
