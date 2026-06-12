package tools_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/starbops/controllerless/internal/tools"
)

func newHelpersReg(t *testing.T) *tools.Registry {
	t.Helper()
	reg := tools.NewRegistry()
	tools.RegisterHelperTools(reg)
	return reg
}

func callHelper(t *testing.T, reg *tools.Registry, name string, args any) any {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	result, err := reg.Call(context.Background(), name, raw)
	if err != nil {
		t.Fatalf("helper %q: %v", name, err)
	}
	return result
}

func TestHelper_TimeNow(t *testing.T) {
	reg := newHelpersReg(t)
	before := time.Now()
	result := callHelper(t, reg, "time.now", map[string]any{})
	after := time.Now()

	ts, ok := result.(time.Time)
	if !ok {
		t.Fatalf("expected time.Time, got %T", result)
	}
	if ts.Before(before) || ts.After(after) {
		t.Fatalf("time.now %v out of range [%v, %v]", ts, before, after)
	}
}

func TestHelper_TimeToUnix(t *testing.T) {
	reg := newHelpersReg(t)
	fixed := time.Unix(1_000_000, 0).UTC()

	result := callHelper(t, reg, "time.toUnix", map[string]any{
		"t": fixed.Format(time.RFC3339Nano),
	})

	unix, ok := result.(int64)
	if !ok {
		t.Fatalf("expected int64, got %T", result)
	}
	if unix != 1_000_000 {
		t.Fatalf("expected 1000000, got %d", unix)
	}
}

func TestHelper_TimeFromUnix(t *testing.T) {
	reg := newHelpersReg(t)

	result := callHelper(t, reg, "time.fromUnix", map[string]any{"unix": int64(1_000_000)})

	ts, ok := result.(time.Time)
	if !ok {
		t.Fatalf("expected time.Time, got %T", result)
	}
	if ts.Unix() != 1_000_000 {
		t.Fatalf("expected unix 1000000, got %d", ts.Unix())
	}
}

func TestHelper_TimeParseDuration(t *testing.T) {
	reg := newHelpersReg(t)

	result := callHelper(t, reg, "time.parseDuration", map[string]any{"s": "2h30m"})

	d, ok := result.(time.Duration)
	if !ok {
		t.Fatalf("expected time.Duration, got %T", result)
	}
	if d != 2*time.Hour+30*time.Minute {
		t.Fatalf("expected 2h30m, got %v", d)
	}
}

func TestHelper_TimeParseDuration_Invalid(t *testing.T) {
	reg := newHelpersReg(t)
	raw, _ := json.Marshal(map[string]any{"s": "not-a-duration"})
	_, err := reg.Call(context.Background(), "time.parseDuration", raw)
	if err == nil {
		t.Fatal("expected error for invalid duration, got nil")
	}
}

func TestHelper_TimeSecondsUntil(t *testing.T) {
	reg := newHelpersReg(t)
	future := time.Now().Add(10 * time.Second)

	result := callHelper(t, reg, "time.secondsUntil", map[string]any{
		"t": future.Format(time.RFC3339Nano),
	})

	secs, ok := result.(int)
	if !ok {
		t.Fatalf("expected int, got %T", result)
	}
	if secs < 0 || secs > 11 {
		t.Fatalf("expected ~10s, got %d", secs)
	}
}

func TestHelper_TimeSince(t *testing.T) {
	reg := newHelpersReg(t)
	past := time.Now().Add(-5 * time.Second)

	result := callHelper(t, reg, "time.since", map[string]any{
		"t": past.Format(time.RFC3339Nano),
	})

	secs, ok := result.(int)
	if !ok {
		t.Fatalf("expected int, got %T", result)
	}
	if secs < 4 || secs > 7 {
		t.Fatalf("expected ~5s, got %d", secs)
	}
}

func TestHelper_CronNextFireTime(t *testing.T) {
	reg := newHelpersReg(t)

	// Every minute. Fire after a specific second.
	after := time.Date(2025, 1, 1, 12, 0, 30, 0, time.UTC)
	result := callHelper(t, reg, "cron.nextFireTime", map[string]any{
		"expr":  "* * * * *",
		"after": after.Format(time.RFC3339Nano),
	})

	ts, ok := result.(time.Time)
	if !ok {
		t.Fatalf("expected time.Time, got %T", result)
	}
	// Next fire should be the start of the next minute.
	expected := time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC)
	if !ts.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, ts)
	}
}

func TestHelper_CronNextFireTime_InvalidExpr(t *testing.T) {
	reg := newHelpersReg(t)
	raw, _ := json.Marshal(map[string]any{
		"expr":  "not-a-cron",
		"after": time.Now().Format(time.RFC3339Nano),
	})
	_, err := reg.Call(context.Background(), "cron.nextFireTime", raw)
	if err == nil {
		t.Fatal("expected error for invalid cron expression, got nil")
	}
}
