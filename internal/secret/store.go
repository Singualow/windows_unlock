package secret

import "errors"

var ErrNotFound = errors.New("secret not found")

type Store interface {
	Put(name string, value []byte) error
	Get(name string) ([]byte, error)
	Delete(name string) error
}

func CredentialName(sid string) string { return "L$ProximityUnlock/Credential/" + sid }
func PresenceName(id string) string    { return "L$ProximityUnlock/Presence/" + id }
func PairingName(id string) string     { return "L$ProximityUnlock/Pairing/" + id }
