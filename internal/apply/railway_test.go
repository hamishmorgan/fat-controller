package apply_test

import (
	"testing"

	"github.com/hamishmorgan/fat-controller/internal/apply"
)

func TestRailwayApplier_ImplementsApplier(t *testing.T) {
	// Compile-time check that RailwayApplier satisfies the Applier interface.
	var _ apply.Applier = (*apply.RailwayApplier)(nil)
}
