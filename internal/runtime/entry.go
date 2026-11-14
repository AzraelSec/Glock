package runtime

import (
	"github.com/AzraelSec/glock/internal/component"
	"github.com/AzraelSec/glock/internal/process"
)

// ComponentEntry pairs a Component definition with its runtime Process.
// It is a runtime view model used by providers; the real runtime will
// resolve Component and Process through separate registries.
type ComponentEntry struct {
	Component component.Component
	Process   *process.Process
}
