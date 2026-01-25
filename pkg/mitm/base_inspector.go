package mitm

import (
	"strings"
)

type Direction int

const (
	DirectionClientToServer Direction = iota
	DirectionServerToClient
)

func (d Direction) String() string {
	switch d {
	case DirectionClientToServer:
		return "client->server"
	case DirectionServerToClient:
		return "server->client"
	default:
		return "unknown"
	}
}

type Inspector interface {
	Name() string
	Inspect(direction Direction, data []byte, hostname string, connectionID, requestID string) ([]byte, error)
	ShouldInspect(hostname string) bool
}

type BaseInspector struct {
	name     string
	hostname string
}

func NewBaseInspector(name, hostname string) *BaseInspector {
	return &BaseInspector{
		name:     name,
		hostname: hostname,
	}
}

func (b *BaseInspector) Name() string {
	return b.name
}

func (b *BaseInspector) ShouldInspect(hostname string) bool {
	if b.hostname == "" {
		return true
	}
	return strings.Contains(hostname, b.hostname)
}
