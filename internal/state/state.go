package state

// Store aggregates all resource-specific storage interfaces.
type Store struct {
	Teams   TeamStore
	Subnets SubnetStore
}
