package applicablemodifiers

import (
	_ "embed"
	"os"
	"testing"
)

//go:embed subsystem_registry_schema.json
var embeddedRegistrySchema []byte

func TestSubsystemRegistrySchemaSyncWithCanonical(t *testing.T) {
	canonical, err := os.ReadFile("../../../../config/applicable-modifiers/schema/subsystems.schema.json")
	if err != nil {
		t.Fatalf("read canonical schema: %v", err)
	}
	if string(embeddedRegistrySchema) != string(canonical) {
		t.Fatalf(
			"services/api/internal/applicablemodifiers/subsystem_registry_schema.json is out of sync "+
				"with config/applicable-modifiers/schema/subsystems.schema.json — "+
				"copy canonical into the embedded path and re-run go generate or the generate script",
		)
	}
}
