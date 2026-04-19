package state

import "errors"

var ErrSubnetNotFound = errors.New("subnet not found")
var ErrNoAvailableSubnets = errors.New("no available subnets")

type SubnetMap map[string]string

// SubnetStore defines the interface for persisting platform state related to subnets.
// This allows us to swap the filesystem-based storage for a database or remote API later.
type SubnetStore interface {
	AllocateSubnet(team string) (string, error)
	GetSubnet(team string) (string, error)
	ListSubnets() (SubnetMap, error)
}
