package testpluginapi

// ServiceName is the exported RPC service name for the v0.1.0 test plugin.
const ServiceName = "TestPlugin"

const (
	MethodAuth         = ServiceName + ".Auth"
	MethodCapabilities = ServiceName + ".Capabilities"
	MethodHeartbeat    = ServiceName + ".Heartbeat"
	MethodEcho         = ServiceName + ".Echo"
	MethodSleep        = ServiceName + ".Sleep"
	MethodCrash        = ServiceName + ".Crash"
	MethodShutdown     = ServiceName + ".Shutdown"
)
