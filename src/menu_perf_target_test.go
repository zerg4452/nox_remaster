package opennox

import (
	"testing"
	"time"
)

func TestMenuPerfWarmupAndMeasurement(t *testing.T) {
	start := time.Unix(100, 0)
	m := menuPerfTarget{started: start}
	for _, sec := range []int{0, 5, 10} {
		if done, err := m.Next(start.Add(time.Duration(sec)*time.Second), menuPerfSample{RenderNS: 7, WaitNS: 11, HD: true}); err != nil || done {
			t.Fatalf("early done=%t err=%v", done, err)
		}
	}
	if len(m.samples) != 0 {
		t.Fatal("warmup included")
	}
	if done, err := m.Next(start.Add(11*time.Second), menuPerfSample{RenderNS: 123, WaitNS: 456, HD: true}); done || err != nil {
		t.Fatal(done, err)
	}
	if len(m.samples) != 1 || m.samples[0].FrameNS != int64(time.Second) || m.samples[0].RenderNS != 123 || m.samples[0].WaitNS != 456 || !m.samples[0].HD {
		t.Fatalf("sample=%+v", m.samples)
	}
	if done, err := m.Next(start.Add(70*time.Second), menuPerfSample{}); !done || err != nil {
		t.Fatalf("done=%t err=%v", done, err)
	}
	if m.elapsed != 60*time.Second {
		t.Fatalf("elapsed=%s", m.elapsed)
	}
}

func TestMenuPerfRejectsNonmonotonicTimeAndUnboundedSamples(t *testing.T) {
	start := time.Unix(100, 0)
	m := menuPerfTarget{started: start, last: start.Add(10 * time.Second)}
	if _, err := m.Next(start.Add(9*time.Second), menuPerfSample{}); err == nil {
		t.Fatal("backward time accepted")
	}
	m = menuPerfTarget{started: start, last: start.Add(10 * time.Second), samples: make([]menuPerfSample, 120000)}
	if _, err := m.Next(start.Add(11*time.Second), menuPerfSample{}); err == nil {
		t.Fatal("sample cap ignored")
	}
}
