package engine

type EventStatus string

const (
	StatusStarted  EventStatus = "started"
	StatusFinished EventStatus = "finished"
	StatusInfo     EventStatus = "info"
	StatusWarning  EventStatus = "warning"
	StatusError    EventStatus = "error"
)

type Event struct {
	Name   string
	Status EventStatus
	Fields map[string]string
}

type Observer interface {
	OnEvent(e Event)
}

type NoopObserver struct{}

func (NoopObserver) OnEvent(Event) {}

// MultiObserver fans out a single event to every underlying observer.
type MultiObserver []Observer

func (m MultiObserver) OnEvent(e Event) {
	for _, o := range m {
		o.OnEvent(e)
	}
}
