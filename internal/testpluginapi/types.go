// Package testpluginapi defines the RPC contract used by the v0.1.0 test plugin.
package testpluginapi

import "time"

// Status reports plugin health at heartbeat time.
type Status string

const (
	StatusStarting   Status = "starting"
	StatusHealthy    Status = "healthy"
	StatusDegraded   Status = "degraded"
	StatusOverloaded Status = "overloaded"
	StatusUnhealthy  Status = "unhealthy"
)

// Empty is a convenience RPC request/response type.
type Empty struct{}

// AuthRequest proves one-time startup token possession.
type AuthRequest struct {
	Token string
}

// AuthResponse reports authenticated plugin identity and generation.
type AuthResponse struct {
	PluginID     string
	Version      string
	GenerationID uint64
}

// CapabilitiesResponse reports plugin identity, generation, and capabilities.
type CapabilitiesResponse struct {
	PluginID     string
	Version      string
	GenerationID uint64
	Capabilities []string
}

// HeartbeatResponse reports liveness and basic health information.
type HeartbeatResponse struct {
	PluginID              string
	Version               string
	GenerationID          uint64
	UptimeSeconds         int64
	Status                Status
	CurrentWorkCount      int
	LastSuccessfulUnixSec int64
	RecentErrorCount      int
}

// EchoRequest sends one message to be echoed back.
type EchoRequest struct {
	Message string
}

// EchoResponse returns the echoed message.
type EchoResponse struct {
	Message string
}

// SleepRequest asks the plugin to sleep for one duration.
type SleepRequest struct {
	Duration time.Duration
}

// CrashRequest requests an immediate process exit with one code.
type CrashRequest struct {
	Code int
}
