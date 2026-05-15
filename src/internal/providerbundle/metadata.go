package providerbundle

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ProviderBundleStatus is an inert validation state for declared provider bundle
// metadata. It is not a broker-emitted surface, not an admission result, and
// not an authority grant.
type ProviderBundleStatus string

const (
	ProviderBundleStatusDeclaredValid       ProviderBundleStatus = "declared_valid"
	ProviderBundleStatusDeclaredUnavailable ProviderBundleStatus = "declared_unavailable"
	ProviderBundleStatusDeclaredInvalid     ProviderBundleStatus = "declared_invalid"
)

// ProviderBundleDigest identifies inert content by digest. It is provenance for
// a later admission layer; the substrate does not interpret descriptor meaning.
type ProviderBundleDigest struct {
	Algorithm string
	Value     string
}

// ProviderBundleAsset is a declared Lua asset path plus digest. The path is a
// metadata fact, not an executable instruction.
type ProviderBundleAsset struct {
	Path   string
	Digest ProviderBundleDigest
}

// ProviderBundleMetadata is a substrate snapshot of provider-owned bundle
// metadata for one authenticated plugin generation.
type ProviderBundleMetadata struct {
	PluginID                  string
	PluginGeneration          int64
	BundleRootPath            string
	ManifestPath              string
	LuaAssets                 []ProviderBundleAsset
	ManifestDigest            ProviderBundleDigest
	SchemaVersion             string
	ValidationStatus          ProviderBundleStatus
	RedactedErrorCode         string
	RedactedErrorMessage      string
	ObservedAt                time.Time
	SubstrateEventCorrelation string
}

// DeclaredProviderBundleCoreDTO is the core-facing projection. Its name and kind
// intentionally say declared metadata so callers cannot confuse it with a
// broker-emitted or admitted surface. It carries provenance and digest facts
// only; host-private paths stay out of the core admission-facing DTO.
type DeclaredProviderBundleCoreDTO struct {
	Kind                      string
	Name                      string
	PluginID                  string
	PluginGeneration          int64
	LuaAssetDigests           []ProviderBundleDigest
	ManifestDigest            ProviderBundleDigest
	SchemaVersion             string
	ValidationStatus          ProviderBundleStatus
	ObservedAt                time.Time
	SubstrateEventCorrelation string
}

// ProviderBundleAdminDTO is an admin-safe inert projection. When path redaction
// is requested, host-private path detail is removed.
type ProviderBundleAdminDTO struct {
	Kind                      string
	Name                      string
	PluginID                  string
	PluginGeneration          int64
	BundleRootPath            string
	ManifestPath              string
	LuaAssets                 []ProviderBundleAsset
	ManifestDigest            ProviderBundleDigest
	SchemaVersion             string
	ValidationStatus          ProviderBundleStatus
	ErrorCode                 string
	ErrorMessage              string
	ObservedAt                time.Time
	SubstrateEventCorrelation string
}

var (
	pluginIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	hexDigest64     = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
	secretLike      = regexp.MustCompile(`(?i)(secret|token|password|passwd|private[ _-]?key|api[ _-]?key|bearer|credential|authority[ _-]?use[ _-]?ref|authority[ _-]?ref|socket|handle|signer|session)`)
)

// Clone returns an independent snapshot of metadata slice fields.
func (m ProviderBundleMetadata) Clone() ProviderBundleMetadata {
	m.LuaAssets = cloneAssets(m.LuaAssets)
	return m
}

// LuaAssetPaths returns a fresh slice of declared Lua asset paths.
func (m ProviderBundleMetadata) LuaAssetPaths() []string {
	paths := make([]string, len(m.LuaAssets))
	for i, asset := range m.LuaAssets {
		paths[i] = asset.Path
	}
	return paths
}

// Validate rejects malformed inert metadata. If expectedGeneration is supplied,
// it must match the metadata generation so stale observations are not accepted.
func (m ProviderBundleMetadata) Validate(expectedGeneration ...int64) error {
	if err := validatePluginID(m.PluginID); err != nil {
		return err
	}
	if m.PluginGeneration <= 0 {
		return fmt.Errorf("plugin generation must be positive")
	}
	if len(expectedGeneration) > 1 {
		return fmt.Errorf("expected generation accepts at most one value")
	}
	if len(expectedGeneration) == 1 && m.PluginGeneration != expectedGeneration[0] {
		return fmt.Errorf("stale provider bundle metadata generation %d does not match expected generation %d", m.PluginGeneration, expectedGeneration[0])
	}
	if strings.TrimSpace(m.BundleRootPath) == "" {
		return fmt.Errorf("bundle root path is required")
	}
	if strings.TrimSpace(m.ManifestPath) == "" {
		return fmt.Errorf("manifest path is required")
	}
	if err := m.ManifestDigest.Validate(); err != nil {
		return fmt.Errorf("manifest digest: %w", err)
	}
	if err := m.ValidationStatus.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(m.SchemaVersion) == "" {
		return fmt.Errorf("schema version is required")
	}
	if m.ObservedAt.IsZero() {
		return fmt.Errorf("observed time is required")
	}
	if strings.TrimSpace(m.SubstrateEventCorrelation) == "" {
		return fmt.Errorf("substrate event correlation id is required")
	}
	if containsSecretLike(m.RedactedErrorCode) || containsSecretLike(m.RedactedErrorMessage) {
		return fmt.Errorf("redacted error fields must not contain secret-like material")
	}
	for i, asset := range m.LuaAssets {
		if strings.TrimSpace(asset.Path) == "" {
			return fmt.Errorf("lua asset %d path is required", i)
		}
		if err := asset.Digest.Validate(); err != nil {
			return fmt.Errorf("lua asset %d digest: %w", i, err)
		}
	}
	return nil
}

// Validate rejects malformed digest facts. v1 metadata only admits sha256
// digests because accepting vague algorithms would make downstream provenance
// comparison dishonest.
func (d ProviderBundleDigest) Validate() error {
	if strings.ToLower(strings.TrimSpace(d.Algorithm)) != "sha256" {
		return fmt.Errorf("digest algorithm must be sha256")
	}
	if !hexDigest64.MatchString(strings.TrimSpace(d.Value)) {
		return fmt.Errorf("digest value must be 64 hex characters")
	}
	return nil
}

// Validate rejects unknown bundle metadata statuses.
func (s ProviderBundleStatus) Validate() error {
	switch s {
	case ProviderBundleStatusDeclaredValid, ProviderBundleStatusDeclaredUnavailable, ProviderBundleStatusDeclaredInvalid:
		return nil
	default:
		return fmt.Errorf("invalid provider bundle validation status %q", s)
	}
}

// CoreDTO returns an inert declared-metadata projection for the broker/core
// boundary. It does not contain authority references, host-private paths, or
// broker-emitted surfaces.
func (m ProviderBundleMetadata) CoreDTO() DeclaredProviderBundleCoreDTO {
	assets := cloneAssets(m.LuaAssets)
	digests := make([]ProviderBundleDigest, 0, len(assets))
	for _, asset := range assets {
		digests = append(digests, asset.Digest)
	}
	return DeclaredProviderBundleCoreDTO{
		Kind:                      "declared_provider_bundle_metadata",
		Name:                      "declared_provider_bundle_metadata",
		PluginID:                  m.PluginID,
		PluginGeneration:          m.PluginGeneration,
		LuaAssetDigests:           digests,
		ManifestDigest:            m.ManifestDigest,
		SchemaVersion:             m.SchemaVersion,
		ValidationStatus:          m.ValidationStatus,
		ObservedAt:                m.ObservedAt,
		SubstrateEventCorrelation: RedactSecretLike(m.SubstrateEventCorrelation),
	}
}

// AdminDTO returns an inert admin projection. If redactPaths is true, absolute
// host paths and secret-like strings are redacted.
func (m ProviderBundleMetadata) AdminDTO(redactPaths bool) ProviderBundleAdminDTO {
	assets := cloneAssets(m.LuaAssets)
	for i := range assets {
		assets[i].Path = RedactAdminString(assets[i].Path, redactPaths)
	}
	return ProviderBundleAdminDTO{
		Kind:                      "declared_provider_bundle_metadata_admin",
		Name:                      "declared_provider_bundle_metadata",
		PluginID:                  RedactSecretLike(m.PluginID),
		PluginGeneration:          m.PluginGeneration,
		BundleRootPath:            RedactAdminString(m.BundleRootPath, redactPaths),
		ManifestPath:              RedactAdminString(m.ManifestPath, redactPaths),
		LuaAssets:                 assets,
		ManifestDigest:            m.ManifestDigest,
		SchemaVersion:             RedactSecretLike(m.SchemaVersion),
		ValidationStatus:          m.ValidationStatus,
		ErrorCode:                 RedactSecretLike(m.RedactedErrorCode),
		ErrorMessage:              RedactSecretLike(m.RedactedErrorMessage),
		ObservedAt:                m.ObservedAt,
		SubstrateEventCorrelation: RedactSecretLike(m.SubstrateEventCorrelation),
	}
}

// RedactSecretLike removes strings that look like credential material.
func RedactSecretLike(s string) string {
	if containsSecretLike(s) {
		return "[redacted]"
	}
	return s
}

// RedactAdminString redacts secret-like content and, when requested, absolute
// host path detail while preserving the final basename when safe.
func RedactAdminString(s string, redactPaths bool) string {
	if RedactSecretLike(s) == "[redacted]" {
		return "[redacted]"
	}
	if !redactPaths || s == "" || !filepath.IsAbs(s) {
		return s
	}
	base := filepath.Base(s)
	if base == "." || base == string(filepath.Separator) || RedactSecretLike(base) == "[redacted]" {
		return "[redacted-path]"
	}
	return "[redacted-path]/" + base
}

func cloneAssets(in []ProviderBundleAsset) []ProviderBundleAsset {
	if in == nil {
		return nil
	}
	out := make([]ProviderBundleAsset, len(in))
	copy(out, in)
	return out
}

func containsSecretLike(s string) bool {
	return secretLike.MatchString(s)
}

func validatePluginID(pluginID string) error {
	if pluginID == "" {
		return fmt.Errorf("plugin id is required")
	}
	if !pluginIDPattern.MatchString(pluginID) {
		return fmt.Errorf("invalid plugin id %q: use only letters, digits, dot, underscore, and dash, and start with a letter or digit", pluginID)
	}
	return nil
}
