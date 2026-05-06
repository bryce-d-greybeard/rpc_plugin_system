package plugin

var serveExampleWithConfig = ServeWithConfig

// Example_minimal shows the smallest useful Go plugin built on the public SDK.
// It stays close to the exported surface and now points at TemplatePlugin as the
// stable skeleton authors should start from.
func Example_minimal() {
	cfg, err := LoadConfigFromEnv()
	if err != nil {
		panic(err)
	}

	p := NewTemplate(cfg, "0.1.0")

	if err := serveExampleWithConfig(cfg, p); err != nil {
		panic(err)
	}
}
