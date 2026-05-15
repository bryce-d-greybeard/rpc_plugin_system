package providerbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultManifestFilename = "manifest.json"

// ProviderBundleLoadOptions identifies one filesystem bundle observation. The
// loader produces inert metadata facts only; it does not execute Lua, interpret
// permissions, mint references, or contact authority owners.
type ProviderBundleLoadOptions struct {
	PluginID                  string
	PluginGeneration          int64
	ExpectedPluginGeneration  int64
	BundleRootPath            string
	ManifestPath              string
	ManifestFilename          string
	ExpectedLuaAssetPaths     []string
	ObservedAt                time.Time
	SubstrateEventCorrelation string
}

// LoadProviderBundleMetadata reads a provider-owned bundle from disk and
// returns inert ProviderBundleMetadata. Failures are represented as
// declared_unavailable or declared_invalid metadata with redacted error facts.
func LoadProviderBundleMetadata(opts ProviderBundleLoadOptions) ProviderBundleMetadata {
	metadata := ProviderBundleMetadata{
		PluginID:                  opts.PluginID,
		PluginGeneration:          opts.PluginGeneration,
		BundleRootPath:            opts.BundleRootPath,
		ObservedAt:                opts.ObservedAt,
		SubstrateEventCorrelation: opts.SubstrateEventCorrelation,
	}

	if err := validateLoadIdentity(opts); err != nil {
		metadata.ManifestPath = inertManifestPath(opts)
		return failMetadata(metadata, ProviderBundleStatusDeclaredInvalid, "malformed_load_identity", "provider bundle load identity is invalid")
	}
	if opts.ExpectedPluginGeneration > 0 && opts.PluginGeneration != opts.ExpectedPluginGeneration {
		metadata.ManifestPath = inertManifestPath(opts)
		return failMetadata(metadata, ProviderBundleStatusDeclaredInvalid, "stale_generation", "plugin generation does not match expected generation")
	}

	root, err := prepareBundleRoot(opts.BundleRootPath)
	if err != nil {
		metadata.ManifestPath = inertManifestPath(opts)
		return failMetadata(metadata, ProviderBundleStatusDeclaredUnavailable, "missing_bundle_root", "bundle root is unavailable")
	}
	metadata.BundleRootPath = root

	manifestPath, err := resolveManifestPath(root, opts.ManifestPath, opts.ManifestFilename)
	if err != nil {
		metadata.ManifestPath = inertManifestPath(opts)
		return failMetadata(metadata, ProviderBundleStatusDeclaredInvalid, "invalid_manifest_path", "manifest path is outside bundle root")
	}
	metadata.ManifestPath = manifestPath

	manifestDigest, manifestBytes, err := digestFile(manifestPath)
	if err != nil {
		code := "unreadable_manifest"
		if os.IsNotExist(err) {
			code = "missing_manifest"
		}
		return failMetadata(metadata, ProviderBundleStatusDeclaredUnavailable, code, "manifest is unavailable")
	}
	metadata.ManifestDigest = manifestDigest

	identity, err := parseManifestIdentity(manifestBytes)
	if err != nil {
		return failMetadata(metadata, ProviderBundleStatusDeclaredInvalid, "malformed_manifest_identity", "manifest identity is invalid")
	}
	if identity.PluginID != "" && identity.PluginID != opts.PluginID {
		return failMetadata(metadata, ProviderBundleStatusDeclaredInvalid, "malformed_manifest_identity", "manifest plugin id does not match")
	}
	if identity.PluginGeneration != nil && *identity.PluginGeneration != opts.PluginGeneration {
		return failMetadata(metadata, ProviderBundleStatusDeclaredInvalid, "malformed_manifest_identity", "manifest plugin generation does not match")
	}
	metadata.SchemaVersion = identity.SchemaVersion

	assets := make([]ProviderBundleAsset, 0, len(opts.ExpectedLuaAssetPaths))
	for _, declaredPath := range opts.ExpectedLuaAssetPaths {
		assetPath, err := resolveBundlePath(root, declaredPath)
		if err != nil {
			metadata.LuaAssets = append(assets, ProviderBundleAsset{Path: declaredPath})
			return failMetadata(metadata, ProviderBundleStatusDeclaredInvalid, "invalid_lua_asset_path", "lua asset path is outside bundle root")
		}
		digest, _, err := digestFile(assetPath)
		if err != nil {
			metadata.LuaAssets = append(assets, ProviderBundleAsset{Path: assetPath})
			code := "unreadable_lua_asset"
			if os.IsNotExist(err) {
				code = "missing_lua_asset"
			}
			return failMetadata(metadata, ProviderBundleStatusDeclaredUnavailable, code, "lua asset is unavailable")
		}
		assets = append(assets, ProviderBundleAsset{Path: assetPath, Digest: digest})
	}

	metadata.LuaAssets = assets
	metadata.ValidationStatus = ProviderBundleStatusDeclaredValid
	return metadata
}

type manifestIdentity struct {
	SchemaVersion    string
	PluginID         string
	PluginGeneration *int64
}

func validateLoadIdentity(opts ProviderBundleLoadOptions) error {
	if err := validatePluginID(opts.PluginID); err != nil {
		return err
	}
	if opts.PluginGeneration <= 0 {
		return fmt.Errorf("plugin generation must be positive")
	}
	if opts.ObservedAt.IsZero() {
		return fmt.Errorf("observed time is required")
	}
	if strings.TrimSpace(opts.SubstrateEventCorrelation) == "" {
		return fmt.Errorf("substrate event correlation id is required")
	}
	return nil
}

func parseManifestIdentity(b []byte) (manifestIdentity, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return manifestIdentity{}, err
	}
	id := manifestIdentity{}
	if err := firstString(raw, &id.SchemaVersion, "schema_version", "schemaVersion"); err != nil {
		return manifestIdentity{}, err
	}
	if strings.TrimSpace(id.SchemaVersion) == "" {
		return manifestIdentity{}, fmt.Errorf("schema version is required")
	}
	if err := firstString(raw, &id.PluginID, "plugin_id", "pluginId", "plugin"); err != nil {
		return manifestIdentity{}, err
	}
	var generation int64
	if ok, err := firstInt64(raw, &generation, "plugin_generation", "pluginGeneration", "generation"); err != nil {
		return manifestIdentity{}, err
	} else if ok {
		id.PluginGeneration = &generation
	}
	return id, nil
}

func firstString(raw map[string]json.RawMessage, dst *string, keys ...string) error {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if err := json.Unmarshal(value, dst); err != nil {
			return err
		}
		return nil
	}
	return nil
}

func firstInt64(raw map[string]json.RawMessage, dst *int64, keys ...string) (bool, error) {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if err := json.Unmarshal(value, dst); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, nil
}

func prepareBundleRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("bundle root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("bundle root is not a directory")
	}
	return resolved, nil
}

func resolveManifestPath(root, manifestPath, manifestFilename string) (string, error) {
	if strings.TrimSpace(manifestPath) != "" {
		return resolveBundlePath(root, manifestPath)
	}
	filename := strings.TrimSpace(manifestFilename)
	if filename == "" {
		filename = defaultManifestFilename
	}
	return resolveBundlePath(root, filename)
}

func resolveBundlePath(root, p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("path is required")
	}
	candidate := p
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	if !pathWithinRoot(root, abs) {
		return "", fmt.Errorf("path escapes bundle root")
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil && !pathWithinRoot(root, resolved) {
		return "", fmt.Errorf("path escapes bundle root")
	}
	return abs, nil
}

func pathWithinRoot(root, path string) bool {
	rootWithSep := root
	if !strings.HasSuffix(rootWithSep, string(filepath.Separator)) {
		rootWithSep += string(filepath.Separator)
	}
	return path == root || strings.HasPrefix(path, rootWithSep)
}

func digestFile(path string) (ProviderBundleDigest, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return ProviderBundleDigest{}, nil, err
	}
	defer f.Close()

	h := sha256.New()
	b, err := io.ReadAll(io.TeeReader(f, h))
	if err != nil {
		return ProviderBundleDigest{}, nil, err
	}
	return ProviderBundleDigest{Algorithm: "sha256", Value: hex.EncodeToString(h.Sum(nil))}, b, nil
}

func failMetadata(metadata ProviderBundleMetadata, status ProviderBundleStatus, code, message string) ProviderBundleMetadata {
	metadata.ValidationStatus = status
	metadata.RedactedErrorCode = RedactSecretLike(code)
	metadata.RedactedErrorMessage = RedactSecretLike(message)
	return metadata
}

func inertManifestPath(opts ProviderBundleLoadOptions) string {
	if strings.TrimSpace(opts.ManifestPath) != "" {
		return opts.ManifestPath
	}
	if strings.TrimSpace(opts.ManifestFilename) != "" {
		return filepath.Join(opts.BundleRootPath, opts.ManifestFilename)
	}
	if strings.TrimSpace(opts.BundleRootPath) != "" {
		return filepath.Join(opts.BundleRootPath, defaultManifestFilename)
	}
	return ""
}
