//go:build !server

package opennox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorldPerfWaitReport(t *testing.T) {
	old, oldHook := worldPerf, worldPerfWait
	defer func() { worldPerf, worldPerfWait = old, oldHook }()
	t.Setenv("NOX_WORLD_HD_FLOORS", "")
	worldPerf.path = filepath.Join(t.TempDir(), "report.json")
	worldPerf.done = false
	worldPerf.target = menuPerfTarget{}
	worldPerf.sample = menuPerfSample{Width: 1280, Height: 720}
	observeWorldPerfWait(time.Second, time.Second)
	if worldPerf.sample.WaitNS != 0 {
		t.Fatal("counted wait before measurement start")
	}
	start := time.Unix(100, 0)
	worldPerf.target.started = start
	worldPerf.target.last = start.Add(10 * time.Second)
	worldPerfWait = observeWorldPerfWait
	worldPerfWait(7*time.Millisecond, 6*time.Millisecond)
	worldPerfWait(5*time.Millisecond, 4*time.Millisecond)
	if _, err := worldPerf.target.Next(start.Add(10*time.Second+33*time.Millisecond), worldPerf.sample); err != nil {
		t.Fatal(err)
	}
	// A new interval starts empty; an unknown request stays unknown if calls mix.
	worldPerf.sample = menuPerfSample{}
	observeWorldPerfWait(time.Millisecond, -1)
	observeWorldPerfWait(time.Millisecond, time.Millisecond)
	if worldPerf.sample.WaitRequestedNS != -1 {
		t.Fatal("unknown target was presented as a known duration")
	}
	finishWorldPerf(nil)
	if worldPerfWait != nil {
		t.Fatal("completed measurement left the wait hook active")
	}
	before := worldPerf.sample
	observeWorldPerfWait(time.Second, time.Second)
	if worldPerf.sample != before {
		t.Fatal("wait changed after completion")
	}
	raw, err := os.ReadFile(worldPerf.path)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Status  string           `json:"status"`
		Timing  string           `json:"limiter_timing"`
		Samples []menuPerfSample `json:"samples"`
	}
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "complete" || report.Timing == "" || len(report.Samples) != 1 {
		t.Fatalf("invalid report: %+v", report)
	}
	s := report.Samples[0]
	if s.WaitNS != int64(12*time.Millisecond) || s.WaitRequestedNS != int64(10*time.Millisecond) || s.FrameNS != int64(33*time.Millisecond) {
		t.Fatalf("interval wait was lost or misattributed: %+v", s)
	}
}

func TestWorldPerfWaitHookOnDisabledLimiter(t *testing.T) {
	oldWorld, oldMenu, oldLimit := worldPerfWait, menuPerfWait, useFrameLimit
	defer func() { worldPerfWait, menuPerfWait, useFrameLimit = oldWorld, oldMenu, oldLimit }()
	menuPerfWait = nil
	useFrameLimit = false
	calls := 0
	worldPerfWait = func(elapsed, requested time.Duration) {
		calls++
		if elapsed < 0 || requested != 0 {
			t.Fatalf("disabled limiter claimed a requested wait: %v/%v", elapsed, requested)
		}
	}
	mainloopFrameLimit()
	if calls != 1 {
		t.Fatalf("limiter observation calls=%d", calls)
	}
}

func TestWorldPerfWaitPacedBeforePresent(t *testing.T) {
	oldWorld, oldMenu, oldLimit, oldPaced := worldPerfWait, menuPerfWait, useFrameLimit, framePacedBeforePresent
	defer func() {
		worldPerfWait, menuPerfWait, useFrameLimit, framePacedBeforePresent = oldWorld, oldMenu, oldLimit, oldPaced
	}()
	menuPerfWait = nil
	calls := 0
	worldPerfWait = func(time.Duration, time.Duration) { calls++ }
	useFrameLimit = false
	framePacedBeforePresent = false
	mainloopPaceBeforePresent()
	if framePacedBeforePresent || calls != 0 {
		t.Fatal("disabled limiter paced before present")
	}
	// A wait already taken before present replaces exactly one loop-end wait.
	framePacedBeforePresent = true
	mainloopFrameLimit()
	if framePacedBeforePresent || calls != 0 {
		t.Fatalf("loop-end limiter waited again after pacing: calls=%d", calls)
	}
	mainloopFrameLimit()
	if calls != 1 {
		t.Fatalf("next loop-end limiter was skipped: calls=%d", calls)
	}
}
