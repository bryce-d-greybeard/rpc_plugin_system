package testpluginapi

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

const BehaviorConfigEnv = "RPC_PLUGIN_SYSTEM_TEST_BEHAVIOR_FILE"
const ShutdownMarkerEnv = "RPC_PLUGIN_SYSTEM_TEST_SHUTDOWN_MARKER_FILE"

// Env describes controllable plugin behavior used by the local failure-point suite.
type Env struct {
	PluginID         string  `json:"plugin_id,omitempty"`
	Version          string  `json:"version,omitempty"`
	GenerationOffset int64   `json:"generation_offset,omitempty"`
	CapabilitiesMode string  `json:"capabilities_mode,omitempty"`
	HeartbeatStatus  Status  `json:"heartbeat_status,omitempty"`
	HeartbeatErrors  int     `json:"heartbeat_errors,omitempty"`
	HeartbeatDelayMS int64   `json:"heartbeat_delay_ms,omitempty"`
	SleepScale       float64 `json:"sleep_scale,omitempty"`
	CrashOnEcho      bool    `json:"crash_on_echo,omitempty"`
	ShutdownDelayMS  int64   `json:"shutdown_delay_ms,omitempty"`
	FailAuth         bool    `json:"fail_auth,omitempty"`
	CloseOnAccept    bool    `json:"close_on_accept,omitempty"`
	CloseOnEcho      bool    `json:"close_on_echo,omitempty"`
}

func (e Env) HeartbeatDelay() time.Duration {
	if e.HeartbeatDelayMS <= 0 {
		return 0
	}
	return time.Duration(e.HeartbeatDelayMS) * time.Millisecond
}

func (e Env) ShutdownDelay() time.Duration {
	if e.ShutdownDelayMS <= 0 {
		return 50 * time.Millisecond
	}
	return time.Duration(e.ShutdownDelayMS) * time.Millisecond
}

func LoadConfig() Env {
	if path := os.Getenv(BehaviorConfigEnv); path != "" {
		data, err := os.ReadFile(path)
		if err == nil {
			var cfg Env
			if json.Unmarshal(data, &cfg) == nil {
				if cfg.Version == "" {
					cfg.Version = "0.1.0"
				}
				if cfg.HeartbeatStatus == "" {
					cfg.HeartbeatStatus = StatusHealthy
				}
				if cfg.SleepScale == 0 {
					cfg.SleepScale = 1
				}
				return cfg
			}
		}
	}
	return LoadEnv()
}

func WriteConfig(path string, cfg Env) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal behavior config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write behavior config: %w", err)
	}
	return nil
}

// LoadEnv reads legacy local plugin-suite behavior controls from environment variables.
func LoadEnv() Env {
	return Env{
		PluginID:         getenvDefault("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_ID", ""),
		Version:          getenvDefault("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_VERSION", "0.1.0"),
		GenerationOffset: getenvInt64("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_GENERATION_OFFSET", 0),
		CapabilitiesMode: getenvDefault("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CAPABILITIES_MODE", "normal"),
		HeartbeatStatus:  Status(getenvDefault("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_STATUS", string(StatusHealthy))),
		HeartbeatErrors:  int(getenvInt64("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_ERRORS", 0)),
		HeartbeatDelayMS: getenvInt64("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_HEARTBEAT_DELAY_MS", 0),
		SleepScale:       getenvFloat64("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_SLEEP_SCALE", 1),
		CrashOnEcho:      getenvBool("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CRASH_ON_ECHO", false),
		ShutdownDelayMS:  getenvInt64("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_SHUTDOWN_DELAY_MS", 50),
		FailAuth:         getenvBool("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_FAIL_AUTH", false),
		CloseOnAccept:    getenvBool("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CLOSE_ON_ACCEPT", false),
		CloseOnEcho:      getenvBool("RPC_PLUGIN_SYSTEM_PLUGIN_BEHAVIOR_CLOSE_ON_ECHO", false),
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
