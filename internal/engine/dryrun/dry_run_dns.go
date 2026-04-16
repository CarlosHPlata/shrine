package dryrun

import (
	"fmt"
	"github.com/CarlosHPlata/shrine/internal/engine"
)

// DryRunDNSBackend implements DNSBackend by printing AdGuard DNS operations.
type DryRunDNSBackend struct{}

func (d *DryRunDNSBackend) WriteRecord(op engine.WriteRecordOp) error {
	fmt.Printf("[DNS]    AddRecord: %s → %s\n", op.Name, op.Value)
	return nil
}

func (d *DryRunDNSBackend) RemoveRecord(team string, name string) error {
	fmt.Printf("[DNS]    RemoveRecord: %s (team=%s)\n", name, team)
	return nil
}
