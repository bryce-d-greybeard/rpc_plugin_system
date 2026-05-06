package testpluginapi

import plugin "rpc_plugin_system/plugin-authoring-sdk/go-runtime/plugin"

const (
	ServiceName        = plugin.ServiceName
	MethodAuth         = plugin.MethodAuth
	MethodCapabilities = plugin.MethodCapabilities
	MethodHeartbeat    = plugin.MethodHeartbeat
	MethodEcho         = plugin.MethodEcho
	MethodSleep        = plugin.MethodSleep
	MethodCrash        = plugin.MethodCrash
	MethodShutdown     = plugin.MethodShutdown
)
