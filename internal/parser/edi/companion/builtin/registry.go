package builtin

import "gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/edi/companion"

// RegisterAll registers all built-in companion guides into the provided registry.
func RegisterAll(registry *companion.Registry) error {
	if registry == nil {
		return nil
	}

	guides := []*companion.CompanionGuide{
		MedicarePartB(),
		BlueCrossBlueShield(),
		UnitedHealthcare(),
	}

	for _, g := range guides {
		if err := registry.Register(g); err != nil {
			return err
		}
	}

	return nil
}
