package state

import "errors"

var ErrSecretNotFound = errors.New("secret not found")

type SecretStore interface {
	GetOrGenerate(team string, key string, length int) (string, bool, error)
	Get(team string, key string) (string, error)
	List(team string) (map[string]string, error)
}
