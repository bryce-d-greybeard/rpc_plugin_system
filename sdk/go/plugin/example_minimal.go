package plugin

import "time"

// Example_minimal shows the smallest useful Go plugin built on the public SDK.
// It is an example function so it stays close to the exported surface and shows
// the intended authoring path without requiring plugin authors to read internals.
func Example_minimal() {
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		panic(err)
	}

	p := &minimalPlugin{
		pluginID:     cfg.PluginID,
		generationID: cfg.GenerationID,
		startedAt:    time.Now(),
	}

	if err := ServeWithConfig(cfg, p); err != nil {
		panic(err)
	}
}

type minimalPlugin struct {
	pluginID     string
	generationID uint64
	startedAt    time.Time
}

func (p *minimalPlugin) Version() string {
	return "0.1.0"
}

func (p *minimalPlugin) Heartbeat(_ Empty, out *HeartbeatResponse) error {
	*out = HeartbeatResponse{
		PluginID:      p.pluginID,
		Version:       p.Version(),
		GenerationID:  p.generationID,
		UptimeSeconds: int64(time.Since(p.startedAt).Seconds()),
		Status:        StatusHealthy,
	}
	return nil
}

func (p *minimalPlugin) Shutdown(_ Empty, _ *Empty) error {
	return nil
}
