package providerbundle

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

const sha256Digest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func validMetadata() ProviderBundleMetadata {
	return ProviderBundleMetadata{
		PluginID:         "provider.echo",
		PluginGeneration: 7,
		BundleRootPath:   "/srv/providers/echo/tool-skills",
		ManifestPath:     "/srv/providers/echo/tool-skills/manifest.json",
		LuaAssets: []ProviderBundleAsset{{
			Path: "/srv/providers/echo/tool-skills/lua/echo.lua",
			Digest: ProviderBundleDigest{
				Algorithm: "sha256",
				Value:     sha256Digest,
			},
		}},
		ManifestDigest: ProviderBundleDigest{
			Algorithm: "sha256",
			Value:     sha256Digest,
		},
		SchemaVersion:             "tool-skills.v1",
		ValidationStatus:          ProviderBundleStatusDeclaredValid,
		RedactedErrorCode:         "",
		RedactedErrorMessage:      "",
		ObservedAt:                time.Unix(1700000000, 0).UTC(),
		SubstrateEventCorrelation: "evt-123",
	}
}

func TestValidateAcceptsValidDeclaredBundleMetadata(t *testing.T) {
	metadata := validMetadata()

	if err := metadata.Validate(7); err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}

	paths := metadata.LuaAssetPaths()
	if got, want := paths[0], "/srv/providers/echo/tool-skills/lua/echo.lua"; got != want {
		t.Fatalf("LuaAssetPaths()[0] = %q, want %q", got, want)
	}
}

func TestValidateRejectsMissingPluginID(t *testing.T) {
	metadata := validMetadata()
	metadata.PluginID = ""

	if err := metadata.Validate(); err == nil || !strings.Contains(err.Error(), "plugin id") {
		t.Fatalf("Validate error = %v, want plugin id rejection", err)
	}
}

func TestValidateRejectsNonPositiveGeneration(t *testing.T) {
	metadata := validMetadata()
	metadata.PluginGeneration = 0

	if err := metadata.Validate(); err == nil || !strings.Contains(err.Error(), "generation") {
		t.Fatalf("Validate error = %v, want generation rejection", err)
	}
}

func TestValidateRejectsMissingBundleAndManifestIdentity(t *testing.T) {
	for name, mutate := range map[string]func(*ProviderBundleMetadata){
		"bundle root": func(m *ProviderBundleMetadata) { m.BundleRootPath = "" },
		"manifest":    func(m *ProviderBundleMetadata) { m.ManifestPath = "" },
	} {
		t.Run(name, func(t *testing.T) {
			metadata := validMetadata()
			mutate(&metadata)

			if err := metadata.Validate(); err == nil {
				t.Fatalf("Validate succeeded, want missing identity rejection")
			}
		})
	}
}

func TestValidateRejectsMalformedDigestAndStatus(t *testing.T) {
	t.Run("manifest digest", func(t *testing.T) {
		metadata := validMetadata()
		metadata.ManifestDigest.Value = "not-hex"

		if err := metadata.Validate(); err == nil || !strings.Contains(err.Error(), "manifest digest") {
			t.Fatalf("Validate error = %v, want manifest digest rejection", err)
		}
	})

	t.Run("asset digest", func(t *testing.T) {
		metadata := validMetadata()
		metadata.LuaAssets[0].Digest.Algorithm = "md5"

		if err := metadata.Validate(); err == nil || !strings.Contains(err.Error(), "lua asset 0 digest") {
			t.Fatalf("Validate error = %v, want asset digest rejection", err)
		}
	})

	t.Run("status", func(t *testing.T) {
		metadata := validMetadata()
		metadata.ValidationStatus = ProviderBundleStatus("admitted_surface")

		if err := metadata.Validate(); err == nil || !strings.Contains(err.Error(), "validation status") {
			t.Fatalf("Validate error = %v, want validation status rejection", err)
		}
	})
}

func TestValidateRejectsStaleExpectedGeneration(t *testing.T) {
	metadata := validMetadata()

	if err := metadata.Validate(8); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("Validate error = %v, want stale generation rejection", err)
	}
}

func TestValidateRejectsMissingObservationAndCorrelationFacts(t *testing.T) {
	for name, mutate := range map[string]func(*ProviderBundleMetadata){
		"observed time": func(m *ProviderBundleMetadata) { m.ObservedAt = time.Time{} },
		"correlation":   func(m *ProviderBundleMetadata) { m.SubstrateEventCorrelation = "" },
	} {
		t.Run(name, func(t *testing.T) {
			metadata := validMetadata()
			mutate(&metadata)

			if err := metadata.Validate(); err == nil {
				t.Fatalf("Validate succeeded, want missing observation/correlation rejection")
			}
		})
	}
}

func TestValidateRejectsSecretLikeRedactedErrorFields(t *testing.T) {
	metadata := validMetadata()
	metadata.RedactedErrorCode = "token_parse_failed"

	if err := metadata.Validate(); err == nil || !strings.Contains(err.Error(), "redacted error") {
		t.Fatalf("Validate error = %v, want redacted error rejection", err)
	}

	metadata = validMetadata()
	metadata.RedactedErrorMessage = "failed with private key material"
	if err := metadata.Validate(); err == nil || !strings.Contains(err.Error(), "redacted error") {
		t.Fatalf("Validate error = %v, want redacted error rejection", err)
	}
}

func TestValidateRejectsMissingLuaAssetPath(t *testing.T) {
	metadata := validMetadata()
	metadata.ValidationStatus = ProviderBundleStatusDeclaredUnavailable
	metadata.RedactedErrorCode = "missing_lua_asset"
	metadata.LuaAssets[0].Path = ""

	if err := metadata.Validate(); err == nil || !strings.Contains(err.Error(), "lua asset 0 path") {
		t.Fatalf("Validate error = %v, want missing lua asset path rejection", err)
	}
}

func TestCloneAndDTOsDoNotLeakMutableAssetSlices(t *testing.T) {
	metadata := validMetadata()

	clone := metadata.Clone()
	clone.LuaAssets[0].Path = "changed.lua"
	if metadata.LuaAssets[0].Path == "changed.lua" {
		t.Fatalf("Clone leaked mutable LuaAssets slice")
	}

	core := metadata.CoreDTO()
	core.LuaAssetDigests[0].Value = "core-changed"
	if metadata.LuaAssets[0].Digest.Value == "core-changed" {
		t.Fatalf("CoreDTO leaked mutable LuaAssets digest slice")
	}

	admin := metadata.AdminDTO(false)
	admin.LuaAssets[0].Path = "admin-changed.lua"
	if metadata.LuaAssets[0].Path == "admin-changed.lua" {
		t.Fatalf("AdminDTO leaked mutable LuaAssets slice")
	}

	paths := metadata.LuaAssetPaths()
	paths[0] = "path-changed.lua"
	if metadata.LuaAssets[0].Path == "path-changed.lua" {
		t.Fatalf("LuaAssetPaths leaked mutable path slice")
	}
}

func TestAdminRedactionRemovesSecretsAndHostPrivatePaths(t *testing.T) {
	metadata := validMetadata()
	metadata.BundleRootPath = "/home/alice/providers/password-bundle"
	metadata.ManifestPath = "/home/alice/providers/manifest.json"
	metadata.LuaAssets[0].Path = "/home/alice/providers/lua/private_key.lua"
	metadata.RedactedErrorCode = "token_parse_failed"
	metadata.RedactedErrorMessage = "failed with bearer token abc123"
	metadata.SubstrateEventCorrelation = "evt-secret-token"

	admin := metadata.AdminDTO(true)

	if admin.BundleRootPath != "[redacted]" {
		t.Fatalf("BundleRootPath = %q, want secret-like redaction", admin.BundleRootPath)
	}
	if got, want := admin.ManifestPath, "[redacted-path]/manifest.json"; got != want {
		t.Fatalf("ManifestPath = %q, want %q", got, want)
	}
	if admin.LuaAssets[0].Path != "[redacted]" {
		t.Fatalf("LuaAssets[0].Path = %q, want secret-like redaction", admin.LuaAssets[0].Path)
	}
	if admin.ErrorCode != "[redacted]" || admin.ErrorMessage != "[redacted]" || admin.SubstrateEventCorrelation != "[redacted]" {
		t.Fatalf("admin secret-like fields not redacted: %#v", admin)
	}
}

func TestCoreDTOExplicitlyDistinguishesDeclaredMetadataFromBrokerSurface(t *testing.T) {
	metadata := validMetadata()
	metadata.SubstrateEventCorrelation = "evt-session-token"

	core := metadata.CoreDTO()

	if got, want := core.Kind, "declared_provider_bundle_metadata"; got != want {
		t.Fatalf("CoreDTO kind = %q, want %q", got, want)
	}
	if got, want := core.Name, "declared_provider_bundle_metadata"; got != want {
		t.Fatalf("CoreDTO name = %q, want %q", got, want)
	}
	if strings.Contains(core.Kind, "broker") || strings.Contains(core.Kind, "admitted") || strings.Contains(core.Kind, "surface") {
		t.Fatalf("CoreDTO kind %q confuses declared metadata with broker/admitted surfaces", core.Kind)
	}
	if len(core.LuaAssetDigests) != 1 || core.LuaAssetDigests[0].Value != sha256Digest || core.ManifestDigest.Value != sha256Digest {
		t.Fatalf("CoreDTO lost digest provenance: %#v", core)
	}
	coreText := fmt.Sprintf("%#v", core)
	for _, forbidden := range []string{"/srv/providers", "echo.lua", "session-token"} {
		if strings.Contains(coreText, forbidden) {
			t.Fatalf("CoreDTO exposed forbidden material %q in %#v", forbidden, core)
		}
	}
}

func TestRedactAdminStringCanLeaveRelativeInertPathsWhenOnlySecretScanRequested(t *testing.T) {
	if got, want := RedactAdminString("tool-skills/lua/echo.lua", true), "tool-skills/lua/echo.lua"; got != want {
		t.Fatalf("relative path redaction = %q, want %q", got, want)
	}
	if got := RedactAdminString("tool-skills/lua/api_key.lua", false); got != "[redacted]" {
		t.Fatalf("secret-like path = %q, want redacted", got)
	}
}
