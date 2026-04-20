package dryrun

import (
	"fmt"
	"io"

	"github.com/CarlosHPlata/shrine/internal/engine"
)

// DryRunDNSBackend implements DNSBackend by printing AdGuard DNS operations.
type DryRunDNSBackend struct {
	Out io.Writer
}

func (d *DryRunDNSBackend) WriteRecord(op engine.WriteRecordOp) error {
	fmt.Fprintf(d.Out, "[DNS]    AddRecord: %s → %s\n", op.Name, op.Value)
	return nil
}

func (d *DryRunDNSBackend) RemoveRecord(team string, name string) error {
	fmt.Fprintf(d.Out, "[DNS]    RemoveRecord: %s (team=%s)\n", name, team)
	return nil
}
