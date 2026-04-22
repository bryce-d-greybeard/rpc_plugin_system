package plugin

import "time"

// TemplatePlugin is the stable minimal plugin skeleton for authors starting a new
// SDK-backed plugin. Copy this shape into a new main package, set Version, and
// optionally add capability interfaces like Echo, Sleep, or Crash.
type TemplatePlugin struct {
	PluginID     string
	GenerationID uint64
	VersionName  string
	StartedAt    time.Time
}

// NewTemplate returns a ready-to-serve minimal plugin skeleton from loaded SDK config.
func NewTemplate(cfg Config, version string) *TemplatePlugin {
	if version == "" {
		version = "0.1.0"
	}
	return &TemplatePlugin{
		PluginID:     cfg.PluginID,
		GenerationID: cfg.GenerationID,
		VersionName:  version,
		StartedAt:    time.Now(),
	}
}

// Version reports the plugin version.
func (p *TemplatePlugin) Version() string {
	return p.VersionName
}

// Heartbeat reports a minimal healthy heartbeat for the current generation.
func (p *TemplatePlugin) Heartbeat(_ Empty, out *HeartbeatResponse) error {
	*out = HeartbeatResponse{
		PluginID:      p.PluginID,
		Version:       p.Version(),
		GenerationID:  p.GenerationID,
		UptimeSeconds: int64(time.Since(p.StartedAt).Seconds()),
		Status:        StatusHealthy,
	}
	return nil
}

// Shutdown accepts a graceful shutdown request.
func (p *TemplatePlugin) Shutdown(_ Empty, _ *Empty) error {
	return nil
}
