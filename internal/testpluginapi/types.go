package testpluginapi

import (
	"time"
	plugin "rpc_plugin_system/sdk/go/plugin"
)

type Status = plugin.Status

const (
	StatusStarting = plugin.StatusStarting
	StatusHealthy = plugin.StatusHealthy
	StatusDegraded = plugin.StatusDegraded
	StatusOverloaded = plugin.StatusOverloaded
	StatusUnhealthy = plugin.StatusUnhealthy
)

type Empty = plugin.Empty
type AuthRequest = plugin.AuthRequest
type AuthResponse = plugin.AuthResponse
type CapabilitiesResponse = plugin.CapabilitiesResponse
type HeartbeatResponse = plugin.HeartbeatResponse
type EchoRequest = plugin.EchoRequest
type EchoResponse = plugin.EchoResponse
type SleepRequest struct { Duration time.Duration }
type CrashRequest = plugin.CrashRequest
