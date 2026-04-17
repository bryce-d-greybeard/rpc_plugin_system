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
				_ = m.log.Write(eventlog.Event{Type: "heartbeat_failed", Plugin: m.cfg.PluginID, Message: err.Error()})
				_ = m.Restart()
			}
		}
	}
}
