package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// RegisterHelperTools registers all pure-function time.* and cron.* helpers.
func RegisterHelperTools(reg *Registry) {
	reg.Register(ToolDef{
		Name:        "time.now",
		Description: "Returns the current UTC time.",
		Schema:      json.RawMessage(`{"type":"object","properties":{}}`),
	}, func(_ context.Context, _ json.RawMessage) (any, error) {
		return time.Now().UTC(), nil
	})

	reg.Register(ToolDef{
		Name:        "time.toUnix",
		Description: "Converts a time to a Unix timestamp (seconds since epoch).",
		Schema: json.RawMessage(`{
			"type":"object",
			"required":["t"],
			"properties":{"t":{"type":"string","format":"date-time"}}
		}`),
	}, func(_ context.Context, args json.RawMessage) (any, error) {
		var p struct {
			T string `json:"t"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		ts, err := time.Parse(time.RFC3339Nano, p.T)
		if err != nil {
			return nil, fmt.Errorf("parse time %q: %w", p.T, err)
		}
		return ts.Unix(), nil
	})

	reg.Register(ToolDef{
		Name:        "time.fromUnix",
		Description: "Converts a Unix timestamp to a time value.",
		Schema: json.RawMessage(`{
			"type":"object",
			"required":["unix"],
			"properties":{"unix":{"type":"integer"}}
		}`),
	}, func(_ context.Context, args json.RawMessage) (any, error) {
		var p struct {
			Unix int64 `json:"unix"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		return time.Unix(p.Unix, 0).UTC(), nil
	})

	reg.Register(ToolDef{
		Name:        "time.parseDuration",
		Description: "Parses a Go duration string (e.g. '2h30m') into a duration.",
		Schema: json.RawMessage(`{
			"type":"object",
			"required":["s"],
			"properties":{"s":{"type":"string"}}
		}`),
	}, func(_ context.Context, args json.RawMessage) (any, error) {
		var p struct {
			S string `json:"s"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		d, err := time.ParseDuration(p.S)
		if err != nil {
			return nil, fmt.Errorf("parse duration %q: %w", p.S, err)
		}
		return d, nil
	})

	reg.Register(ToolDef{
		Name:        "time.secondsUntil",
		Description: "Returns the number of whole seconds until the given time.",
		Schema: json.RawMessage(`{
			"type":"object",
			"required":["t"],
			"properties":{"t":{"type":"string","format":"date-time"}}
		}`),
	}, func(_ context.Context, args json.RawMessage) (any, error) {
		var p struct {
			T string `json:"t"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		ts, err := time.Parse(time.RFC3339Nano, p.T)
		if err != nil {
			return nil, fmt.Errorf("parse time %q: %w", p.T, err)
		}
		return int(time.Until(ts).Seconds()), nil
	})

	reg.Register(ToolDef{
		Name:        "time.since",
		Description: "Returns the number of whole seconds since the given time.",
		Schema: json.RawMessage(`{
			"type":"object",
			"required":["t"],
			"properties":{"t":{"type":"string","format":"date-time"}}
		}`),
	}, func(_ context.Context, args json.RawMessage) (any, error) {
		var p struct {
			T string `json:"t"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		ts, err := time.Parse(time.RFC3339Nano, p.T)
		if err != nil {
			return nil, fmt.Errorf("parse time %q: %w", p.T, err)
		}
		return int(time.Since(ts).Seconds()), nil
	})

	reg.Register(ToolDef{
		Name:        "cron.nextFireTime",
		Description: "Computes the next fire time for a cron expression after a given time.",
		Schema: json.RawMessage(`{
			"type":"object",
			"required":["expr","after"],
			"properties":{
				"expr":{"type":"string","description":"Standard 5-field cron expression"},
				"after":{"type":"string","format":"date-time"}
			}
		}`),
	}, func(_ context.Context, args json.RawMessage) (any, error) {
		var p struct {
			Expr  string `json:"expr"`
			After string `json:"after"`
		}
		if err := json.Unmarshal(args, &p); err != nil {
			return nil, err
		}
		after, err := time.Parse(time.RFC3339Nano, p.After)
		if err != nil {
			return nil, fmt.Errorf("parse after time %q: %w", p.After, err)
		}
		schedule, err := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow).Parse(p.Expr)
		if err != nil {
			return nil, fmt.Errorf("parse cron expression %q: %w", p.Expr, err)
		}
		return schedule.Next(after), nil
	})
}
