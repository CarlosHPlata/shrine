package secrets

// SecretsPlugin is the provider-agnostic contract for a secrets vault backend.
// Implementations must be safe to call with a nil receiver via IsActive().
type SecretsPlugin interface {
	// IsActive returns true when the plugin is configured and ready.
	IsActive() bool
	// GetSecret fetches the secret at path and returns its plaintext value.
	// path is an opaque string whose format is defined by the implementation;
	// for Infisical it is "project/environment/secret-name".
	// Errors must include the path but MUST NOT include the secret value.
	GetSecret(path string) (string, error)
}
