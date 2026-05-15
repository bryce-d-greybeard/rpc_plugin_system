package providerbundle

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadProviderBundleMetadataValidComputesDigestsAndBindsIdentity(t *testing.T) {
	root := t.TempDir()
	manifest := []byte(`{"schema_version":"tool-skills.v1","plugin_id":"provider.echo","plugin_generation":7,"permissions":{"filesystem":"/secret/token"}}`)
	asset := []byte(`error("must not execute")
-- permissions stay inert
`)
	writeTestFile(t, filepath.Join(root, "manifest.json"), manifest, 0o644)
	writeTestFile(t, filepath.Join(root, "lua", "echo.lua"), asset, 0o644)
	observed := time.Unix(1700000000, 0).UTC()

	metadata := LoadProviderBundleMetadata(ProviderBundleLoadOptions{
		PluginID:                  "provider.echo",
		PluginGeneration:          7,
		ExpectedPluginGeneration:  7,
		BundleRootPath:            root,
		ManifestFilename:          "manifest.json",
		ExpectedLuaAssetPaths:     []string{"lua/echo.lua"},
		ObservedAt:                observed,
		SubstrateEventCorrelation: "evt-123",
	})

	if metadata.ValidationStatus != ProviderBundleStatusDeclaredValid {
		t.Fatalf("status = %q, want valid; metadata=%#v", metadata.ValidationStatus, metadata)
	}
	if metadata.PluginID != "provider.echo" || metadata.PluginGeneration != 7 || !metadata.ObservedAt.Equal(observed) || metadata.SubstrateEventCorrelation != "evt-123" {
		t.Fatalf("identity/provenance not bound: %#v", metadata)
	}
	if metadata.SchemaVersion != "tool-skills.v1" {
		t.Fatalf("schema version = %q", metadata.SchemaVersion)
	}
	if got, want := metadata.ManifestDigest.Value, sha256Hex(manifest); got != want {
		t.Fatalf("manifest digest = %q, want %q", got, want)
	}
	if len(metadata.LuaAssets) != 1 {
		t.Fatalf("LuaAssets length = %d", len(metadata.LuaAssets))
	}
	if got, want := metadata.LuaAssets[0].Path, filepath.Join(root, "lua", "echo.lua"); got != want {
		t.Fatalf("asset path = %q, want %q", got, want)
	}
	if got, want := metadata.LuaAssets[0].Digest.Value, sha256Hex(asset); got != want {
		t.Fatalf("asset digest = %q, want %q", got, want)
	}
	if err := metadata.Validate(7); err != nil {
		t.Fatalf("valid loaded metadata did not validate: %v", err)
	}
}

func TestLoadProviderBundleMetadataMissingRootAndManifestFailClosedWithoutSecretLeak(t *testing.T) {
	observed := time.Unix(1700000000, 0).UTC()
	missingRoot := filepath.Join(t.TempDir(), "missing-token-root")

	metadata := LoadProviderBundleMetadata(ProviderBundleLoadOptions{
		PluginID:                  "provider.echo",
		PluginGeneration:          7,
		BundleRootPath:            missingRoot,
		ManifestFilename:          "manifest.json",
		ObservedAt:                observed,
		SubstrateEventCorrelation: "evt-123",
	})
	assertClosedWithoutSecretLeak(t, metadata, ProviderBundleStatusDeclaredUnavailable, "missing_bundle_root")

	root := t.TempDir()
	metadata = LoadProviderBundleMetadata(ProviderBundleLoadOptions{
		PluginID:                  "provider.echo",
		PluginGeneration:          7,
		BundleRootPath:            root,
		ManifestFilename:          "missing-token-manifest.json",
		ObservedAt:                observed,
		SubstrateEventCorrelation: "evt-123",
	})
	assertClosedWithoutSecretLeak(t, metadata, ProviderBundleStatusDeclaredUnavailable, "missing_manifest")

	unreadableManifest := filepath.Join(root, "manifest-as-directory.json")
	if err := os.Mkdir(unreadableManifest, 0o755); err != nil {
		t.Fatalf("Mkdir unreadable manifest stand-in: %v", err)
	}
	metadata = LoadProviderBundleMetadata(ProviderBundleLoadOptions{
		PluginID:                  "provider.echo",
		PluginGeneration:          7,
		BundleRootPath:            root,
		ManifestFilename:          "manifest-as-directory.json",
		ObservedAt:                observed,
		SubstrateEventCorrelation: "evt-123",
	})
	assertClosedWithoutSecretLeak(t, metadata, ProviderBundleStatusDeclaredUnavailable, "unreadable_manifest")
}

func TestLoadProviderBundleMetadataMissingAndUnreadableAssetsFailClosedWithoutSecretLeak(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "manifest.json"), []byte(`{"schema_version":"tool-skills.v1","plugin_id":"provider.echo"}`), 0o644)

	metadata := LoadProviderBundleMetadata(ProviderBundleLoadOptions{
		PluginID:                  "provider.echo",
		PluginGeneration:          7,
		BundleRootPath:            root,
		ManifestPath:              filepath.Join(root, "manifest.json"),
		ExpectedLuaAssetPaths:     []string{"lua/api_key_missing.lua"},
		ObservedAt:                time.Unix(1700000000, 0).UTC(),
		SubstrateEventCorrelation: "evt-123",
	})

	assertClosedWithoutSecretLeak(t, metadata, ProviderBundleStatusDeclaredUnavailable, "missing_lua_asset")
	if len(metadata.LuaAssets) != 1 || !strings.HasSuffix(metadata.LuaAssets[0].Path, filepath.Join("lua", "api_key_missing.lua")) {
		t.Fatalf("missing asset fact not preserved: %#v", metadata.LuaAssets)
	}

	if err := os.MkdirAll(filepath.Join(root, "lua", "directory.lua"), 0o755); err != nil {
		t.Fatalf("Mkdir unreadable asset stand-in: %v", err)
	}
	metadata = LoadProviderBundleMetadata(ProviderBundleLoadOptions{
		PluginID:                  "provider.echo",
		PluginGeneration:          7,
		BundleRootPath:            root,
		ManifestPath:              filepath.Join(root, "manifest.json"),
		ExpectedLuaAssetPaths:     []string{"lua/directory.lua"},
		ObservedAt:                time.Unix(1700000000, 0).UTC(),
		SubstrateEventCorrelation: "evt-123",
	})
	assertClosedWithoutSecretLeak(t, metadata, ProviderBundleStatusDeclaredUnavailable, "unreadable_lua_asset")
}

func TestLoadProviderBundleMetadataRejectsMalformedLoadIdentityBeforeFilesystem(t *testing.T) {
	for name, mutate := range map[string]func(*ProviderBundleLoadOptions){
		"bad plugin id":       func(o *ProviderBundleLoadOptions) { o.PluginID = "../bad" },
		"zero generation":     func(o *ProviderBundleLoadOptions) { o.PluginGeneration = 0 },
		"missing observed":    func(o *ProviderBundleLoadOptions) { o.ObservedAt = time.Time{} },
		"missing correlation": func(o *ProviderBundleLoadOptions) { o.SubstrateEventCorrelation = "" },
	} {
		t.Run(name, func(t *testing.T) {
			opts := ProviderBundleLoadOptions{
				PluginID:                  "provider.echo",
				PluginGeneration:          7,
				BundleRootPath:            filepath.Join(t.TempDir(), "missing-root"),
				ManifestFilename:          "manifest.json",
				ExpectedLuaAssetPaths:     []string{"lua/echo.lua"},
				ObservedAt:                time.Unix(1700000000, 0).UTC(),
				SubstrateEventCorrelation: "evt-123",
			}
			mutate(&opts)

			metadata := LoadProviderBundleMetadata(opts)

			assertClosedWithoutSecretLeak(t, metadata, ProviderBundleStatusDeclaredInvalid, "malformed_load_identity")
			if metadata.ManifestDigest.Value != "" || len(metadata.LuaAssets) != 0 {
				t.Fatalf("malformed load identity should not read files or assets: %#v", metadata)
			}
		})
	}
}

func TestLoadProviderBundleMetadataStaleExpectedGenerationFailsClosed(t *testing.T) {
	root := t.TempDir()

	metadata := LoadProviderBundleMetadata(ProviderBundleLoadOptions{
		PluginID:                  "provider.echo",
		PluginGeneration:          7,
		ExpectedPluginGeneration:  8,
		BundleRootPath:            root,
		ManifestFilename:          "manifest.json",
		ObservedAt:                time.Unix(1700000000, 0).UTC(),
		SubstrateEventCorrelation: "evt-123",
	})

	assertClosedWithoutSecretLeak(t, metadata, ProviderBundleStatusDeclaredInvalid, "stale_generation")
	if metadata.ManifestDigest.Value != "" || len(metadata.LuaAssets) != 0 {
		t.Fatalf("stale load should not read files or assets: %#v", metadata)
	}
}

func TestLoadProviderBundleMetadataRejectsPathTraversalAndOutsideBundleAssets(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, filepath.Join(root, "manifest.json"), []byte(`{"schema_version":"tool-skills.v1","plugin_id":"provider.echo"}`), 0o644)
	writeTestFile(t, filepath.Join(outside, "escape.lua"), []byte(`return "outside"`), 0o644)
	if err := os.Symlink(filepath.Join(outside, "escape.lua"), filepath.Join(root, "symlink.lua")); err != nil {
		t.Fatalf("Symlink outside asset: %v", err)
	}

	for name, assetPath := range map[string]string{
		"relative traversal": filepath.Join("..", "escape.lua"),
		"absolute outside":   filepath.Join(outside, "escape.lua"),
		"symlink outside":    "symlink.lua",
	} {
		t.Run(name, func(t *testing.T) {
			metadata := LoadProviderBundleMetadata(ProviderBundleLoadOptions{
				PluginID:                  "provider.echo",
				PluginGeneration:          7,
				BundleRootPath:            root,
				ManifestFilename:          "manifest.json",
				ExpectedLuaAssetPaths:     []string{assetPath},
				ObservedAt:                time.Unix(1700000000, 0).UTC(),
				SubstrateEventCorrelation: "evt-123",
			})

			assertClosedWithoutSecretLeak(t, metadata, ProviderBundleStatusDeclaredInvalid, "invalid_lua_asset_path")
		})
	}
}

func TestLoadProviderBundleMetadataRejectsMalformedManifestIdentity(t *testing.T) {
	for name, manifest := range map[string]string{
		"malformed json":       `{`,
		"missing schema":       `{"plugin_id":"provider.echo"}`,
		"wrong plugin id":      `{"schema_version":"tool-skills.v1","plugin_id":"other.provider"}`,
		"wrong generation":     `{"schema_version":"tool-skills.v1","plugin_id":"provider.echo","plugin_generation":8}`,
		"non-string plugin id": `{"schema_version":"tool-skills.v1","plugin_id":7}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, filepath.Join(root, "manifest.json"), []byte(manifest), 0o644)

			metadata := LoadProviderBundleMetadata(ProviderBundleLoadOptions{
				PluginID:                  "provider.echo",
				PluginGeneration:          7,
				BundleRootPath:            root,
				ManifestFilename:          "manifest.json",
				ObservedAt:                time.Unix(1700000000, 0).UTC(),
				SubstrateEventCorrelation: "evt-123",
			})

			assertClosedWithoutSecretLeak(t, metadata, ProviderBundleStatusDeclaredInvalid, "malformed_manifest_identity")
		})
	}
}

func TestLoadProviderBundleMetadataDoesNotExecuteLuaOrInterpretPermissions(t *testing.T) {
	root := t.TempDir()
	manifest := []byte(`{"schema_version":"tool-skills.v1","plugin_id":"provider.echo","permissions":{"credential":"bearer-token"},"descriptor":{"permissions":["filesystem:*"]}}`)
	asset := []byte(`os.execute("touch should-not-exist")
error("execution would fail")
`)
	writeTestFile(t, filepath.Join(root, "manifest.json"), manifest, 0o644)
	writeTestFile(t, filepath.Join(root, "lua", "echo.lua"), asset, 0o644)

	metadata := LoadProviderBundleMetadata(ProviderBundleLoadOptions{
		PluginID:                  "provider.echo",
		PluginGeneration:          7,
		BundleRootPath:            root,
		ManifestFilename:          "manifest.json",
		ExpectedLuaAssetPaths:     []string{"lua/echo.lua"},
		ObservedAt:                time.Unix(1700000000, 0).UTC(),
		SubstrateEventCorrelation: "evt-123",
	})

	if metadata.ValidationStatus != ProviderBundleStatusDeclaredValid {
		t.Fatalf("status = %q, want valid", metadata.ValidationStatus)
	}
	if metadata.ManifestDigest.Value != sha256Hex(manifest) || metadata.LuaAssets[0].Digest.Value != sha256Hex(asset) {
		t.Fatalf("loader did more/less than digest inert content: %#v", metadata)
	}
	if metadata.RedactedErrorCode != "" || metadata.RedactedErrorMessage != "" {
		t.Fatalf("permissions content should not be interpreted as error/authority: %#v", metadata)
	}
	if _, err := os.Stat(filepath.Join(root, "lua", "should-not-exist")); !os.IsNotExist(err) {
		t.Fatalf("lua appears to have been executed, stat err=%v", err)
	}
}

func writeTestFile(t *testing.T, path string, b []byte, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, b, perm); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func assertClosedWithoutSecretLeak(t *testing.T, metadata ProviderBundleMetadata, status ProviderBundleStatus, code string) {
	t.Helper()
	if metadata.ValidationStatus != status {
		t.Fatalf("status = %q, want %q; metadata=%#v", metadata.ValidationStatus, status, metadata)
	}
	if metadata.RedactedErrorCode != code {
		t.Fatalf("error code = %q, want %q", metadata.RedactedErrorCode, code)
	}
	if metadata.RedactedErrorMessage == "" {
		t.Fatalf("missing redacted error message")
	}
	for _, s := range []string{metadata.RedactedErrorCode, metadata.RedactedErrorMessage} {
		if containsSecretLike(s) {
			t.Fatalf("secret-like material leaked in redacted error field: %q", s)
		}
	}
}
