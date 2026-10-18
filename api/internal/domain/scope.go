package domain

// Scope limits what an API key may do.
type Scope string

const (
	// ScopeAdmin allows everything, including managing keys and endpoints.
	ScopeAdmin Scope = "admin"
	// ScopeIngest allows sending events and nothing else.
	ScopeIngest Scope = "ingest"
	// ScopeRead allows GET requests only.
	ScopeRead Scope = "read"
)

// Valid reports whether s is a known scope.
func (s Scope) Valid() bool {
	return s == ScopeAdmin || s == ScopeIngest || s == ScopeRead
}

// Allows reports whether the scope set permits an action that needs required.
// Admin permits every action.
func Allows(scopes []Scope, required Scope) bool {
	for _, s := range scopes {
		if s == ScopeAdmin || s == required {
			return true
		}
	}
	return false
}
