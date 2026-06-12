package skill

import (
	"fmt"

	"github.com/google/cel-go/cel"
)

// EvalTrigger compiles and evaluates a CEL expression against the given
// Kubernetes object (unstructured). An empty expression always returns true.
//
// The top-level keys of obj (e.g., "spec", "metadata") are bound as
// variables so expressions like "spec.suspend != true" work directly.
func EvalTrigger(expr string, obj map[string]any) (bool, error) {
	if expr == "" {
		return true, nil
	}

	// Declare each top-level key as a dynamic variable.
	var opts []cel.EnvOption
	for k := range obj {
		opts = append(opts, cel.Variable(k, cel.DynType))
	}

	env, err := cel.NewEnv(opts...)
	if err != nil {
		return false, fmt.Errorf("skill: cel env: %w", err)
	}

	ast, iss := env.Compile(expr)
	if iss.Err() != nil {
		return false, fmt.Errorf("skill: cel compile %q: %w", expr, iss.Err())
	}

	prg, err := env.Program(ast)
	if err != nil {
		return false, fmt.Errorf("skill: cel program: %w", err)
	}

	// obj provides the activation; nil obj is treated as empty.
	activation := map[string]any{}
	for k, v := range obj {
		activation[k] = v
	}

	out, _, err := prg.Eval(activation)
	if err != nil {
		return false, fmt.Errorf("skill: cel eval: %w", err)
	}

	result, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("skill: cel expression must return bool, got %T", out.Value())
	}
	return result, nil
}
