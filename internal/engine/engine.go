package engine

type Engine struct {
	Container ContainerBackend
	Routing   RoutingBackend
	DNS       DNSBackend
}
