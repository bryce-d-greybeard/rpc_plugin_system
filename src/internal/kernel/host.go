package kernel

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"rpc_plugin_system/internal/providerbundle"
	"rpc_plugin_system/test/testpluginapi"
)

// PluginConfig defines one supervised plugin instance managed by a Host.
type PluginConfig struct {
	PluginID   string
	PluginPath string

	// ProviderBundleMetadata is inert declared bundle metadata to project onto
	// admin/core snapshots when it matches the current plugin generation.
	ProviderBundleMetadata *providerbundle.ProviderBundleMetadata

	// ProviderBundleMetadataRefresh is an explicit inert config refresh path for
	// declared bundle metadata keyed by authenticated plugin generation.
	ProviderBundleMetadataRefresh *ProviderBundleMetadataRefreshConfig
}

// HostConfig defines the multi-plugin kernel host configuration.
type HostConfig struct {
	RuntimeDir     string
	EventLogPath   string
	DialTimeout    time.Duration
	CallTimeout    time.Duration
	HeartbeatEvery time.Duration
	Plugins        []PluginConfig

	// DisableProviderBundleAdminPathRedaction leaves host-private paths visible
	// on admin DTOs. The default is redaction, because admin status often crosses
	// process and operator boundaries.
	DisableProviderBundleAdminPathRedaction bool
}

// Host supervises multiple plugin managers and exposes registry-style access.
type Host struct {
	mu       sync.RWMutex
	managers map[string]*Manager
	plugins  []string
}

// HostState is the top-level multi-plugin state view.
type HostState struct {
	Plugins       []State
	CapabilityMap map[string][]string
	Routes        []Route
}

// CoreSnapshot is the core-facing host projection. It is intentionally separate
// from HostState so admin surfaces cannot accidentally expose raw core-only
// metadata.
type CoreSnapshot struct {
	Plugins []CoreState
}

// Route describes one explicit direct-routing target known to the host.
type Route struct {
	PluginID     string
	Capabilities []string
	Healthy      bool
	GenerationID uint64
	Mode         string
}

var routeHostCall = func(h *Host, pluginID string, call RoutedCall, arg any) (RoutedResponse, error) {
	return h.Call(pluginID, call, arg)
}

// CapabilityMap returns the current capability-to-plugin map.
func (h *Host) CapabilityMap() map[string][]string {
	return h.State().CapabilityMap
}

// Routes returns the current direct-routing table in stable plugin-id order.
func (h *Host) Routes() []Route {
	return h.State().Routes
}

// Plugin returns one plugin state by plugin id.
func (h *Host) Plugin(pluginID string) (State, error) {
	manager, err := h.Manager(pluginID)
	if err != nil {
		return State{}, err
	}
	return manager.State(), nil
}

// NewHost constructs one multi-plugin host backed by one manager per plugin.
func NewHost(cfg HostConfig) (*Host, error) {
	if cfg.RuntimeDir == "" {
		return nil, fmt.Errorf("runtime dir is required")
	}
	if len(cfg.Plugins) == 0 {
		return nil, fmt.Errorf("at least one plugin is required")
	}
	host := &Host{managers: map[string]*Manager{}}
	seen := map[string]struct{}{}
	for _, plugin := range cfg.Plugins {
		if err := ValidatePluginID(plugin.PluginID); err != nil {
			return nil, err
		}
		if plugin.PluginPath == "" {
			return nil, fmt.Errorf("plugin path is required")
		}
		if _, ok := seen[plugin.PluginID]; ok {
			return nil, fmt.Errorf("duplicate plugin id: %s", plugin.PluginID)
		}
		seen[plugin.PluginID] = struct{}{}
		manager, err := New(Config{
			RuntimeDir:                              pluginRuntimeDir(cfg.RuntimeDir, plugin.PluginID),
			PluginPath:                              plugin.PluginPath,
			PluginID:                                plugin.PluginID,
			DialTimeout:                             cfg.DialTimeout,
			CallTimeout:                             cfg.CallTimeout,
			HeartbeatEvery:                          cfg.HeartbeatEvery,
			EventLogPath:                            pluginEventLogPath(cfg.RuntimeDir, plugin.PluginID),
			ProviderBundleMetadata:                  plugin.ProviderBundleMetadata,
			ProviderBundleMetadataRefresh:           cloneProviderBundleMetadataRefresh(plugin.ProviderBundleMetadataRefresh),
			DisableProviderBundleAdminPathRedaction: cfg.DisableProviderBundleAdminPathRedaction,
		})
		if err != nil {
			for _, existing := range host.managers {
				_ = existing.Close()
			}
			return nil, fmt.Errorf("new manager for %s: %w", plugin.PluginID, err)
		}
		host.managers[plugin.PluginID] = manager
		host.plugins = append(host.plugins, plugin.PluginID)
	}
	sort.Strings(host.plugins)
	return host, nil
}

// Close shuts down all managers owned by the host.
func (h *Host) Close() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var firstErr error
	for _, pluginID := range h.plugins {
		if err := h.managers[pluginID].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// StartAll starts all configured plugins.
func (h *Host) StartAll() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	started := make([]string, 0, len(h.plugins))
	for _, pluginID := range h.plugins {
		if err := h.managers[pluginID].Start(); err != nil {
			for _, startedID := range started {
				_ = h.managers[startedID].Kill()
			}
			return fmt.Errorf("start plugin %s: %w", pluginID, err)
		}
		started = append(started, pluginID)
	}
	return nil
}

// Manager returns one manager by plugin id.
func (h *Host) Manager(pluginID string) (*Manager, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	manager, ok := h.managers[pluginID]
	if !ok {
		return nil, fmt.Errorf("unknown plugin id: %s", pluginID)
	}
	return manager, nil
}

// State returns a full host view including per-plugin states and capability map.
func (h *Host) State() HostState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := HostState{
		Plugins:       make([]State, 0, len(h.plugins)),
		CapabilityMap: map[string][]string{},
		Routes:        make([]Route, 0, len(h.plugins)),
	}
	for _, pluginID := range h.plugins {
		state := h.managers[pluginID].State()
		out.Plugins = append(out.Plugins, state)
		out.Routes = append(out.Routes, Route{
			PluginID:     state.PluginID,
			Capabilities: append([]string(nil), state.Capabilities...),
			Healthy:      state.Healthy,
			GenerationID: state.GenerationID,
			Mode:         "direct-plugin-id",
		})
		for _, cap := range state.Capabilities {
			out.CapabilityMap[cap] = append(out.CapabilityMap[cap], pluginID)
		}
	}
	for cap := range out.CapabilityMap {
		sort.Strings(out.CapabilityMap[cap])
	}
	return out
}

// States returns the current states in stable plugin-id order.
func (h *Host) States() []State {
	return h.State().Plugins
}

// CoreSnapshot returns core-facing declared metadata facts in stable plugin-id
// order. It is inert substrate state only; it does not admit bundle surfaces or
// mint executable authority.
func (h *Host) CoreSnapshot() CoreSnapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := CoreSnapshot{Plugins: make([]CoreState, 0, len(h.plugins))}
	for _, pluginID := range h.plugins {
		out.Plugins = append(out.Plugins, h.managers[pluginID].CoreState())
	}
	return out
}

// RestartPlugin restarts exactly one plugin and returns its new state.
func (h *Host) RestartPlugin(pluginID string) (State, error) {
	out, err := routeHostCall(h, pluginID, RoutedCallRestart, nil)
	if err != nil {
		return State{}, err
	}
	state, ok := out.Body.(State)
	if !ok {
		return State{}, fmt.Errorf("restart routed response type mismatch")
	}
	return state, nil
}

// RoutedCall represents one explicit direct routed operation by plugin id.
type RoutedCall string

const (
	RoutedCallHeartbeat RoutedCall = "heartbeat"
	RoutedCallEcho      RoutedCall = "echo"
	RoutedCallRestart   RoutedCall = "restart"
)

// RouteTarget resolves one plugin id into its current route entry and manager.
func (h *Host) RouteTarget(pluginID string) (Route, *Manager, error) {
	manager, err := h.Manager(pluginID)
	if err != nil {
		return Route{}, nil, err
	}
	state := manager.State()
	return Route{
		PluginID:     state.PluginID,
		Capabilities: append([]string(nil), state.Capabilities...),
		Healthy:      state.Healthy,
		GenerationID: state.GenerationID,
		Mode:         "direct-plugin-id",
	}, manager, nil
}

// Echo routes one echo request directly to the target plugin id.
func (h *Host) Echo(pluginID, message string) (string, error) {
	_, manager, err := h.RouteTarget(pluginID)
	if err != nil {
		return "", err
	}
	return manager.Echo(message)
}

// Heartbeat routes one heartbeat request directly to the target plugin id.
func (h *Host) Heartbeat(pluginID string) (testpluginapi.HeartbeatResponse, error) {
	_, manager, err := h.RouteTarget(pluginID)
	if err != nil {
		return testpluginapi.HeartbeatResponse{}, err
	}
	return manager.Heartbeat()
}

// RoutedResponse captures the result of one routed operation.
type RoutedResponse struct {
	Route Route
	Call  RoutedCall
	Body  any
}

// RoutedRequest is the generic direct-routing request shape.
type RoutedRequest struct {
	PluginID string
	Call     RoutedCall
	Arg      any
}

// Call routes one supported direct operation to the target plugin id.
func (h *Host) Call(pluginID string, call RoutedCall, arg any) (RoutedResponse, error) {
	return h.CallRequest(RoutedRequest{PluginID: pluginID, Call: call, Arg: arg})
}

// CallRequest routes one supported direct request through the generic request shape.
func (h *Host) CallRequest(req RoutedRequest) (RoutedResponse, error) {
	route, manager, err := h.RouteTarget(req.PluginID)
	if err != nil {
		return RoutedResponse{}, err
	}
	switch req.Call {
	case RoutedCallHeartbeat:
		out, err := manager.Heartbeat()
		if err != nil {
			return RoutedResponse{}, err
		}
		return RoutedResponse{Route: route, Call: req.Call, Body: out}, nil
	case RoutedCallEcho:
		message, _ := req.Arg.(string)
		out, err := manager.Echo(message)
		if err != nil {
			return RoutedResponse{}, err
		}
		return RoutedResponse{Route: route, Call: req.Call, Body: testpluginapi.EchoResponse{Message: out}}, nil
	case RoutedCallRestart:
		if err := manager.Restart(); err != nil {
			return RoutedResponse{}, err
		}
		return RoutedResponse{Route: route, Call: req.Call, Body: manager.State()}, nil
	default:
		return RoutedResponse{}, fmt.Errorf("unsupported routed call: %s", req.Call)
	}
}

// MonitorLoop runs all manager monitor loops until the context ends.
func (h *Host) MonitorLoop(ctx context.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, pluginID := range h.plugins {
		go h.managers[pluginID].MonitorLoop(ctx)
	}
}

func cloneProviderBundleMetadataRefresh(in *ProviderBundleMetadataRefreshConfig) *ProviderBundleMetadataRefreshConfig {
	if in == nil {
		return nil
	}
	out := &ProviderBundleMetadataRefreshConfig{}
	if in.ByGeneration != nil {
		out.ByGeneration = make(map[int64]providerbundle.ProviderBundleMetadata, len(in.ByGeneration))
		for generation, metadata := range in.ByGeneration {
			out.ByGeneration[generation] = metadata.Clone()
		}
	}
	return out
}

func pluginRuntimeDir(root, pluginID string) string {
	return filepath.Join(root, pluginID)
}

func pluginEventLogPath(root, pluginID string) string {
	return filepath.Join(root, pluginID, "events.jsonl")
}
