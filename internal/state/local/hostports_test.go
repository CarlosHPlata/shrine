package local

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/CarlosHPlata/shrine/internal/state"
)

// fakeHostPortFile is an in-memory stand-in for hostports.txt so unit tests
// never touch the real filesystem.
type fakeHostPortFile struct {
	content  []byte
	exists   bool
	writes   int
	writeErr error
}

func (f *fakeHostPortFile) read(string) ([]byte, error) {
	if !f.exists {
		return nil, os.ErrNotExist
	}
	return f.content, nil
}

func (f *fakeHostPortFile) write(_ string, data []byte) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.content = append([]byte(nil), data...)
	f.exists = true
	f.writes++
	return nil
}

func newTestHostPortStore(t *testing.T, file *fakeHostPortFile, reserved []int) state.HostPortStore {
	t.Helper()
	s, err := newHostPortStoreWithFileOps("/state", reserved, file.read, file.write)
	if err != nil {
		t.Fatalf("newHostPortStoreWithFileOps failed: %v", err)
	}
	return s
}

func TestHostPortStore_LoadForgivingFormat(t *testing.T) {
	file := &fakeHostPortFile{
		exists: true,
		content: []byte(`
# comment line
media/jellyfin=30000
  # indented comment
ops/dashboard=8080 # inline comment

malformed-line
ops/bad=not-a-port
`),
	}
	s := newTestHostPortStore(t, file, nil)

	ports, err := s.ListHostPorts()
	if err != nil {
		t.Fatalf("ListHostPorts failed: %v", err)
	}
	want := state.HostPortMap{"media/jellyfin": 30000, "ops/dashboard": 8080}
	if len(ports) != len(want) {
		t.Errorf("got %d entries, want %d: %v", len(ports), len(want), ports)
	}
	for k, v := range want {
		if ports[k] != v {
			t.Errorf("entry %q: got %d, want %d", k, ports[k], v)
		}
	}
}

func TestHostPortStore_MissingFileIsEmptyStore(t *testing.T) {
	s := newTestHostPortStore(t, &fakeHostPortFile{}, nil)
	ports, err := s.ListHostPorts()
	if err != nil {
		t.Fatalf("ListHostPorts failed: %v", err)
	}
	if len(ports) != 0 {
		t.Errorf("missing file should mean empty store, got %v", ports)
	}
}

func TestHostPortStore_AllocateFirstFreeAndPersist(t *testing.T) {
	file := &fakeHostPortFile{}
	s := newTestHostPortStore(t, file, nil)

	port, err := s.AllocateHostPort("demo", "api")
	if err != nil {
		t.Fatalf("AllocateHostPort failed: %v", err)
	}
	if port != 30000 {
		t.Errorf("first allocation: got %d, want 30000", port)
	}
	if !strings.Contains(string(file.content), "demo/api=30000") {
		t.Errorf("allocation should be persisted, file content: %q", file.content)
	}
}

func TestHostPortStore_AllocateIsIdempotent(t *testing.T) {
	file := &fakeHostPortFile{}
	s := newTestHostPortStore(t, file, nil)

	first, err := s.AllocateHostPort("demo", "api")
	if err != nil {
		t.Fatalf("AllocateHostPort failed: %v", err)
	}
	writesAfterFirst := file.writes

	second, err := s.AllocateHostPort("demo", "api")
	if err != nil {
		t.Fatalf("idempotent AllocateHostPort failed: %v", err)
	}
	if first != second {
		t.Errorf("idempotent allocation returned different port: %d vs %d", second, first)
	}
	if file.writes != writesAfterFirst {
		t.Errorf("idempotent allocation should not rewrite the file (writes %d -> %d)", writesAfterFirst, file.writes)
	}
}

func TestHostPortStore_AllocateSkipsReservedAndPersisted(t *testing.T) {
	file := &fakeHostPortFile{
		exists:  true,
		content: []byte("media/jellyfin=30001\n"),
	}
	s := newTestHostPortStore(t, file, []int{30000, 30002})

	port, err := s.AllocateHostPort("demo", "api")
	if err != nil {
		t.Fatalf("AllocateHostPort failed: %v", err)
	}
	if port != 30003 {
		t.Errorf("allocation should skip reserved (30000, 30002) and persisted (30001): got %d, want 30003", port)
	}
}

func TestHostPortStore_ClaimRecordsExplicitPort(t *testing.T) {
	file := &fakeHostPortFile{}
	s := newTestHostPortStore(t, file, nil)

	if err := s.ClaimHostPort("demo", "web", 8080); err != nil {
		t.Fatalf("ClaimHostPort failed: %v", err)
	}
	if !strings.Contains(string(file.content), "demo/web=8080") {
		t.Errorf("claim should be persisted, file content: %q", file.content)
	}

	writes := file.writes
	if err := s.ClaimHostPort("demo", "web", 8080); err != nil {
		t.Fatalf("idempotent ClaimHostPort failed: %v", err)
	}
	if file.writes != writes {
		t.Errorf("re-claiming the same port should not rewrite the file")
	}
}

func TestHostPortStore_ClaimConflictWithOtherApp(t *testing.T) {
	s := newTestHostPortStore(t, &fakeHostPortFile{}, nil)

	if err := s.ClaimHostPort("demo", "web", 8080); err != nil {
		t.Fatalf("ClaimHostPort failed: %v", err)
	}
	err := s.ClaimHostPort("demo", "other", 8080)
	if !errors.Is(err, state.ErrHostPortTaken) {
		t.Errorf("claiming another app's port: got %v, want ErrHostPortTaken", err)
	}
}

func TestHostPortStore_ClaimReservedPortRejected(t *testing.T) {
	s := newTestHostPortStore(t, &fakeHostPortFile{}, []int{443})

	err := s.ClaimHostPort("demo", "web", 443)
	if !errors.Is(err, state.ErrHostPortTaken) {
		t.Errorf("claiming a reserved port: got %v, want ErrHostPortTaken", err)
	}
}

func TestHostPortStore_ClaimOverwriteReleasesOldPort(t *testing.T) {
	s := newTestHostPortStore(t, &fakeHostPortFile{}, nil)

	allocated, err := s.AllocateHostPort("demo", "api")
	if err != nil {
		t.Fatalf("AllocateHostPort failed: %v", err)
	}

	// Switch demo/api from automatic to explicit: the old automatic port must
	// become free again.
	if err := s.ClaimHostPort("demo", "api", 8080); err != nil {
		t.Fatalf("ClaimHostPort overwrite failed: %v", err)
	}
	got, err := s.GetHostPort("demo", "api")
	if err != nil || got != 8080 {
		t.Fatalf("after overwrite: got (%d, %v), want (8080, nil)", got, err)
	}

	reallocated, err := s.AllocateHostPort("demo", "fresh")
	if err != nil {
		t.Fatalf("AllocateHostPort after overwrite failed: %v", err)
	}
	if reallocated != allocated {
		t.Errorf("old automatic port %d should be free after overwrite, next allocation got %d", allocated, reallocated)
	}
}

func TestHostPortStore_GetHostPort(t *testing.T) {
	s := newTestHostPortStore(t, &fakeHostPortFile{}, nil)

	if _, err := s.GetHostPort("demo", "ghost"); !errors.Is(err, state.ErrHostPortNotFound) {
		t.Errorf("absent entry: got %v, want ErrHostPortNotFound", err)
	}

	if err := s.ClaimHostPort("demo", "web", 8080); err != nil {
		t.Fatalf("ClaimHostPort failed: %v", err)
	}
	got, err := s.GetHostPort("demo", "web")
	if err != nil || got != 8080 {
		t.Errorf("got (%d, %v), want (8080, nil)", got, err)
	}
}

func TestHostPortStore_ReleaseIsIdempotent(t *testing.T) {
	file := &fakeHostPortFile{}
	s := newTestHostPortStore(t, file, nil)

	if err := s.ReleaseHostPort("demo", "ghost"); err != nil {
		t.Errorf("releasing an absent entry should be a soft success, got: %v", err)
	}
	if file.writes != 0 {
		t.Errorf("releasing an absent entry should not write the file")
	}

	port, _ := s.AllocateHostPort("demo", "api")
	if err := s.ReleaseHostPort("demo", "api"); err != nil {
		t.Fatalf("ReleaseHostPort failed: %v", err)
	}
	if _, err := s.GetHostPort("demo", "api"); !errors.Is(err, state.ErrHostPortNotFound) {
		t.Errorf("entry should be gone after release, got: %v", err)
	}

	// The released port is allocatable again.
	again, err := s.AllocateHostPort("demo", "other")
	if err != nil {
		t.Fatalf("AllocateHostPort after release failed: %v", err)
	}
	if again != port {
		t.Errorf("released port %d should be reallocatable, got %d", port, again)
	}
}

func TestHostPortStore_ReleaseTeamHostPorts(t *testing.T) {
	s := newTestHostPortStore(t, &fakeHostPortFile{}, nil)

	_, _ = s.AllocateHostPort("demo", "api")
	_ = s.ClaimHostPort("demo", "web", 8080)
	_ = s.ClaimHostPort("media", "jellyfin", 8090)

	if err := s.ReleaseTeamHostPorts("demo"); err != nil {
		t.Fatalf("ReleaseTeamHostPorts failed: %v", err)
	}

	ports, _ := s.ListHostPorts()
	if len(ports) != 1 || ports["media/jellyfin"] != 8090 {
		t.Errorf("only media/jellyfin should remain, got %v", ports)
	}

	if err := s.ReleaseTeamHostPorts("demo"); err != nil {
		t.Errorf("idempotent ReleaseTeamHostPorts failed: %v", err)
	}
}

func TestHostPortStore_Exhaustion(t *testing.T) {
	s := newTestHostPortStore(t, &fakeHostPortFile{}, nil)

	for i := 30000; i <= 32767; i++ {
		if _, err := s.AllocateHostPort("demo", fmt.Sprintf("app-%d", i)); err != nil {
			t.Fatalf("allocation %d failed: %v", i, err)
		}
	}

	_, err := s.AllocateHostPort("demo", "one-too-many")
	if !errors.Is(err, state.ErrNoAvailableHostPorts) {
		t.Errorf("expected ErrNoAvailableHostPorts, got %v", err)
	}
	if _, getErr := s.GetHostPort("demo", "one-too-many"); !errors.Is(getErr, state.ErrHostPortNotFound) {
		t.Error("failed exhaustion allocation must leave no entry behind")
	}
}

func TestHostPortStore_SaveFailureRollsBack(t *testing.T) {
	file := &fakeHostPortFile{writeErr: errors.New("disk full")}
	s := newTestHostPortStore(t, file, nil)

	if _, err := s.AllocateHostPort("demo", "api"); err == nil {
		t.Fatal("allocation should fail when the save fails")
	}
	if _, err := s.GetHostPort("demo", "api"); !errors.Is(err, state.ErrHostPortNotFound) {
		t.Error("failed save must roll back the in-memory allocation")
	}

	// Once saving works again the same port is allocated cleanly.
	file.writeErr = nil
	port, err := s.AllocateHostPort("demo", "api")
	if err != nil {
		t.Fatalf("allocation after recovery failed: %v", err)
	}
	if port != 30000 {
		t.Errorf("rolled-back port should be reusable: got %d, want 30000", port)
	}
}

func TestHostPortStore_SaveFormatSortedByKey(t *testing.T) {
	file := &fakeHostPortFile{}
	s := newTestHostPortStore(t, file, nil)

	_ = s.ClaimHostPort("zeta", "app", 9000)
	_ = s.ClaimHostPort("alpha", "app", 9001)

	want := "alpha/app=9001\nzeta/app=9000\n"
	if string(file.content) != want {
		t.Errorf("file should be sorted by key:\ngot:  %q\nwant: %q", file.content, want)
	}
}

func TestHostPortStore_ListReturnsDefensiveCopy(t *testing.T) {
	s := newTestHostPortStore(t, &fakeHostPortFile{}, nil)
	_ = s.ClaimHostPort("demo", "web", 8080)

	ports, _ := s.ListHostPorts()
	ports["demo/web"] = 1

	got, _ := s.GetHostPort("demo", "web")
	if got != 8080 {
		t.Error("ListHostPorts must return a defensive copy")
	}
}

func TestHostPortStore_ReservedPortsNeverPersisted(t *testing.T) {
	file := &fakeHostPortFile{}
	s := newTestHostPortStore(t, file, []int{80, 443})

	_ = s.ClaimHostPort("demo", "web", 8080)
	if strings.Contains(string(file.content), "80\n") && strings.Contains(string(file.content), "443") {
		t.Errorf("reserved ports must not appear in the file, content: %q", file.content)
	}
	ports, _ := s.ListHostPorts()
	if len(ports) != 1 {
		t.Errorf("reserved ports must not appear in listings, got %v", ports)
	}
}
