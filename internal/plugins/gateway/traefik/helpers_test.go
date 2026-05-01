package traefik

import "github.com/CarlosHPlata/shrine/internal/engine"

// recordingObserver collects events emitted during a call.
type recordingObserver struct {
	events []engine.Event
}

func (r *recordingObserver) OnEvent(e engine.Event) {
	r.events = append(r.events, e)
}
