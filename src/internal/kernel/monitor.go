package kernel

import (
	"context"
	"time"

	"rpc_plugin_system/internal/eventlog"
)

// MonitorLoop polls plugin heartbeat until the context is canceled.
// When a heartbeat fails, it records the failure and attempts a restart.
func (m *Manager) MonitorLoop(ctx context.Context) {
	ticker := time.NewTicker(m.cfg.HeartbeatEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := m.Heartbeat(); err != nil {
				select {
				case <-m.closedCh:
					return
				default:
				}
				m.logEvent(eventlog.Event{
					Level:     eventlog.LevelWarn,
					Component: eventlog.ComponentKernel,
					Event:     eventlog.EventHeartbeatFailed,
					PluginID:  m.cfg.PluginID,
					Message:   "monitor observed heartbeat failure and will restart plugin",
					Error:     err.Error(),
					Reason:    "monitor loop heartbeat check",
				})
				if restartErr := m.Restart(); restartErr != nil {
					m.logEvent(eventlog.Event{
						Level:     eventlog.LevelError,
						Component: eventlog.ComponentKernel,
						Event:     eventlog.EventRestartFailed,
						PluginID:  m.cfg.PluginID,
						Message:   "monitor restart attempt failed",
						Error:     restartErr.Error(),
						Reason:    "monitor loop heartbeat recovery",
					})
				}
			}
		}
	}
}
