package api

import (
	"errors"
	"net"
)

type AuthType int

const (
	DevAlwaysTrue AuthType = iota
	DevAlwaysFalse
	BearerToken
	BasicAuth
)

func isPrivateIP(ip net.IP) bool {
	privateIPBlocks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.1/30",
	}
	for _, block := range privateIPBlocks {
		_, subnet, _ := net.ParseCIDR(block)
		if subnet.Contains(ip) {
			return true
		}
	}
	return false
}

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

