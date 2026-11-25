//go:build integration

package testutils

import "context"

// WatchedContainer remembers a container's ID so a test can tell, after each
// deploy, whether Shrine recreated the container or left it alone.
type WatchedContainer struct {
	tc     *TestCase
	name   string
	lastID string
}

// WatchContainer starts watching an existing container from its current ID.
func (tc *TestCase) WatchContainer(name string) *WatchedContainer {
	tc.t.Helper()
	w := &WatchedContainer{tc: tc, name: name}
	w.lastID = w.currentID()
	return w
}

// AssertRecreated fails unless the container was replaced since the last check.
func (w *WatchedContainer) AssertRecreated() *WatchedContainer {
	w.tc.t.Helper()
	if id := w.currentID(); id == w.lastID {
		w.tc.t.Fatalf("container %q should have been recreated, id stayed %s", w.name, id)
	} else {
		w.lastID = id
	}
	return w
}

// AssertNotRecreated fails if the container was replaced since the last check.
func (w *WatchedContainer) AssertNotRecreated() *WatchedContainer {
	w.tc.t.Helper()
	if id := w.currentID(); id != w.lastID {
		w.tc.t.Fatalf("container %q should have been left alone, id changed %s -> %s", w.name, w.lastID, id)
	}
	return w
}

func (w *WatchedContainer) currentID() string {
	w.tc.t.Helper()
	info, err := w.tc.DockerClient.ContainerInspect(context.Background(), w.name)
	if err != nil {
		w.tc.t.Fatalf("inspecting container %q: %v", w.name, err)
	}
	return info.ID
}
