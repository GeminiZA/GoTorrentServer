package api

import "errors"

type AuthType int

const (
	DevAlwaysTrue AuthType = iota
	DevAlwaysFalse
	BearerToken
	BasicAuth
)

func Authenticate(authType AuthType) error {
	switch authType {
	case DevAlwaysFalse:
		return errors.New("Authentication Failed: Dev")
	case DevAlwaysTrue:
		return nil
	case BearerToken:
		return errors.New("Authentication Failed: Not implemented")
	case BasicAuth:
		return errors.New("Authentication Failed: Not implemented")
	default:
		return errors.New("Authentication Failed: Not specified")
	}
}