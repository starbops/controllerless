package tools

import (
	"context"
	"encoding/json"
)

// DoneSignal is returned by the "done" tool to terminate the reconcile session successfully.
type DoneSignal struct {
	Summary string
}

// RequeueSignal is returned by the "requeueAfter" tool to schedule a future re-enqueue
// and terminate the current reconcile session.
type RequeueSignal struct {
	Seconds int
	Reason  string
}

// RegisterMetaTools registers the done and requeueAfter control-flow tools.
func RegisterMetaTools(reg *Registry) {
	reg.Register(ToolDef{
		Name:        "done",
		Description: "Terminates the reconcile session successfully. Call when the desired state is already achieved or all work is complete.",
		Schema: json.RawMessage(`{
			"type":"object",
			"required":["summary"],
			"properties":{
				"summary":{"type":"string","description":"Short explanation of why reconciliation is complete"}
			}
		}`),
	}, func(_ context.Context, args json.RawMessage) (any, error) {
		var p struct {
			Summary string `json:"summary"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		return DoneSignal{Summary: p.Summary}, nil
	})

	reg.Register(ToolDef{
		Name:        "requeueAfter",
		Description: "Schedules a re-enqueue after the given number of seconds and terminates the current session.",
		Schema: json.RawMessage(`{
			"type":"object",
			"required":["seconds","reason"],
			"properties":{
				"seconds":{"type":"integer","minimum":0},
				"reason":{"type":"string"}
			}
		}`),
	}, func(_ context.Context, args json.RawMessage) (any, error) {
		var p struct {
			Seconds int    `json:"seconds"`
			Reason  string `json:"reason"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		return RequeueSignal{Seconds: p.Seconds, Reason: p.Reason}, nil
	})
}

// ErrTerminal is a sentinel used to check if a result is a terminal signal.
// The agentic loop detects DoneSignal/RequeueSignal via type assertion, not error wrapping.
var ErrTerminal = terminalErr{}

type terminalErr struct{}

func (terminalErr) Error() string { return "terminal signal" }
