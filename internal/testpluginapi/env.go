package testpluginapi

import (
	"os"
	"strconv"
	"time"
)

// Env describes controllable plugin behavior used by the local failure-point suite.
type Env struct {
	PluginID            string
	Version             string
	GenerationOffset    int64
	CapabilitiesMode    string
	HeartbeatStatus     Status
	HeartbeatErrors     int
	SleepScale          float64
	CrashOnEcho         bool
	ShutdownDelay       time.Duration
	FailAuth   bool
	CloseOnAccept       bool
	CloseOnEcho         bool
}

// LoadEnv reads local plugin-suite behavior controls from environment variables.
func LoadEnv() Env {
	return Env{
		PluginID:          getenvDefault("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_ID", ""),
		Version:           getenvDefault("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_VERSION", "0.1.0"),
		GenerationOffset:  getenvInt64("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_GENERATION_OFFSET", 0),
		CapabilitiesMode:  getenvDefault("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CAPABILITIES_MODE", "normal"),
		HeartbeatStatus:   Status(getenvDefault("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_STATUS", string(StatusHealthy))),
		HeartbeatErrors:   int(getenvInt64("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_ERRORS", 0)),
		SleepScale:        getenvFloat64("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_SLEEP_SCALE", 1),
		CrashOnEcho:       getenvBool("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CRASH_ON_ECHO", false),
		ShutdownDelay:     getenvDuration("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_SHUTDOWN_DELAY_MS", 50*time.Millisecond),
		FailAuth: getenvBool("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_FAIL_AUTH", false),
		CloseOnAccept:     getenvBool("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CLOSE_ON_ACCEPT", false),
		CloseOnEcho:       getenvBool("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CLOSE_ON_ECHO", false),
	}
}

func getenvDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getenvInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvFloat64(key string, fallback float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	ms := getenvInt64(key, int64(fallback/time.Millisecond))
	return time.Duration(ms) * time.Millisecond
}
