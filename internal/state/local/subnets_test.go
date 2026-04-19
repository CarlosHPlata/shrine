package local

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/state"
)

func TestSubnetStore_Load(t *testing.T) {
	tmpDir := t.TempDir()
	filepath := filepath.Join(tmpDir, "subnets.txt")

	data := `
# This is a comment
team-a=10.100.5.0/24
  # indented comment
team-b=10.100.6.0/24 # inline comment

invalid-line
team-c=not-a-cidr
`
	if err := os.WriteFile(filepath, []byte(data), 0644); err != nil {
		t.Fatalf("failed to setup test file: %v", err)
	}

	store, err := NewSubnetStore(tmpDir)
	if err != nil {
		t.Fatalf("NewSubnetStore failed: %v", err)
	}

	// Cast to concrete type to check internal maps
	s := store.(*SubnetStore)

	expectedSubnets := map[string]string{
		"team-a": "10.100.5.0/24",
		"team-b": "10.100.6.0/24",
	}

	if len(s.subnets) != len(expectedSubnets) {
		t.Errorf("got %d subnets, want %d", len(s.subnets), len(expectedSubnets))
	}

	for team, cidr := range expectedSubnets {
		if got := s.subnets[team]; got != cidr {
			t.Errorf("team %s: got %q, want %q", team, got, cidr)
		}
	}

	expectedTaken := []uint8{5, 6}
	for _, octet := range expectedTaken {
		if _, ok := s.taken[octet]; !ok {
			t.Errorf("octet %d should be marked as taken", octet)
		}
	}
}

func TestSubnetStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	
	// 1. Create store and add some data
	store, err := NewSubnetStore(tmpDir)
	if err != nil {
		t.Fatalf("NewSubnetStore failed: %v", err)
	}
	
	s := store.(*SubnetStore)
	s.subnets["team-x"] = "10.100.10.0/24"
	s.taken[10] = struct{}{}

	// 2. Save
	if err := s.save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// 3. Create a new store instance and check if it loads the data
	store2, err := NewSubnetStore(tmpDir)
	if err != nil {
		t.Fatalf("NewSubnetStore (re-load) failed: %v", err)
	}

	s2 := store2.(*SubnetStore)
	if got := s2.subnets["team-x"]; got != "10.100.10.0/24" {
		t.Errorf("re-loaded store: got %q, want %q", got, "10.100.10.0/24")
	}
	if _, ok := s2.taken[10]; !ok {
		t.Error("re-loaded store: octet 10 not marked as taken")
	}
}

func TestSubnetStore_Interface(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSubnetStore(tmpDir)
	if err != nil {
		t.Fatalf("NewSubnetStore failed: %v", err)
	}

	// 1. Successful Allocation
	cidr1, err := store.AllocateSubnet("team-a")
	if err != nil {
		t.Fatalf("AllocateSubnet failed: %v", err)
	}
	if cidr1 != "10.100.5.0/24" {
		t.Errorf("got %q, want %q", cidr1, "10.100.5.0/24")
	}

	// 2. Idempotency
	cidr2, err := store.AllocateSubnet("team-a")
	if err != nil {
		t.Fatalf("AllocateSubnet idempotent call failed: %v", err)
	}
	if cidr1 != cidr2 {
		t.Errorf("idempotent call returned different CIDR: %q vs %q", cidr2, cidr1)
	}

	// 3. GetSubnet
	got, err := store.GetSubnet("team-a")
	if err != nil {
		t.Fatalf("GetSubnet failed: %v", err)
	}
	if got != cidr1 {
		t.Errorf("GetSubnet: got %q, want %q", got, cidr1)
	}

	_, err = store.GetSubnet("non-existent")
	if !errors.Is(err, state.ErrSubnetNotFound) {
		t.Errorf("GetSubnet for non-existent team: got error %v, want %v", err, state.ErrSubnetNotFound)
	}

	// 4. ListSubnets
	subnets, err := store.ListSubnets()
	if err != nil {
		t.Fatalf("ListSubnets failed: %v", err)
	}
	if len(subnets) != 1 || subnets["team-a"] != cidr1 {
		t.Errorf("ListSubnets: got %v, want map[team-a:%s]", subnets, cidr1)
	}
}

func TestSubnetStore_DefensiveCopy(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSubnetStore(tmpDir)
	if err != nil {
		t.Fatalf("NewSubnetStore failed: %v", err)
	}

	store.AllocateSubnet("team-a")

	subnets, _ := store.ListSubnets()
	subnets["team-a"] = "MODIFIED"

	// Check if internal state changed
	got, _ := store.GetSubnet("team-a")
	if got == "MODIFIED" {
		t.Error("ListSubnets returned reference to internal map, expected defensive copy")
	}
}

func TestSubnetStore_Exhaustion(t *testing.T) {
	tmpDir := t.TempDir()
	store, err := NewSubnetStore(tmpDir)
	if err != nil {
		t.Fatalf("NewSubnetStore failed: %v", err)
	}

	// Fill up all subnets
	for i := 5; i <= 255; i++ {
		team := fmt.Sprintf("team-%d", i)
		_, err := store.AllocateSubnet(team)
		if err != nil {
			t.Fatalf("failed to allocate subnet %d: %v", i, err)
		}
	}

	// Next allocation should fail
	_, err = store.AllocateSubnet("one-too-many")
	if !errors.Is(err, state.ErrNoAvailableSubnets) {
		t.Errorf("expected ErrNoAvailableSubnets, got %v", err)
	}
}
