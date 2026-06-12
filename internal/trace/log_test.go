package trace_test

import (
	"testing"

	"github.com/starbops/controllerless/internal/trace"
)

func TestInit_noPanic(t *testing.T) {
	trace.Init()
}

func TestInit_idempotent(t *testing.T) {
	trace.Init()
	trace.Init()
}
