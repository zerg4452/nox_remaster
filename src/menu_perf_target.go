package opennox

import (
	"errors"
	"time"
)

type menuPerfSample struct {
	FrameNS         int64 `json:"frame_ns"`
	RenderNS        int64 `json:"render_ns"`
	WaitNS          int64 `json:"limiter_wait_ns"`
	WaitRequestedNS int64 `json:"limiter_requested_ns,omitempty"`
	HD              bool  `json:"hd"`
	Width           int   `json:"output_width"`
	Height          int   `json:"output_height"`
	WindowWidth     int   `json:"window_width"`
	WindowHeight    int   `json:"window_height"`
	Filtering       bool  `json:"filtering"`
}
type menuPerfTarget struct {
	started, last, measureStart time.Time
	elapsed                     time.Duration
	samples                     []menuPerfSample
}

func (m *menuPerfTarget) Next(now time.Time, s menuPerfSample) (bool, error) {
	if now.Before(m.started) || (!m.last.IsZero() && !now.After(m.last)) {
		return false, errors.New("non-monotonic menu performance clock")
	}
	if !m.last.IsZero() && !m.last.Before(m.started.Add(10*time.Second)) {
		if len(m.samples) >= 120000 {
			return false, errors.New("menu performance sample limit exceeded")
		}
		if m.measureStart.IsZero() {
			m.measureStart = m.last
		}
		s.FrameNS = int64(now.Sub(m.last))
		m.samples = append(m.samples, s)
		m.elapsed = now.Sub(m.measureStart)
	}
	m.last = now
	return m.elapsed >= 60*time.Second, nil
}
