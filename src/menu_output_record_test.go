package opennox

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/noxworld-dev/opennox-lib/noximage"
)

func TestMenuOutputPixelsAndOwnership(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "capture")
	r, err := newMenuOutputRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	im := &noximage.Image16{Rect: image.Rect(2, 3, 4, 5), Stride: 3, Pix: []uint16{0x1234, 0x5678, 0xeeee, 0xabcd, 0x7fff, 0xeeee}}
	for n := uint64(40); n < 43; n++ {
		if err := r.Submit(menuOutputFrame{Frame: n, HD: true}, im); err != nil {
			t.Fatal(err)
		}
	}
	for i := range im.Pix {
		im.Pix[i] = 0
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "frames.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 24 {
		t.Fatalf("bytes=%d", len(b))
	}
	for i, v := range []uint16{0x1234, 0x5678, 0xabcd, 0x7fff} {
		if got := binary.LittleEndian.Uint16(b[i*2:]); got != v {
			t.Fatalf("pixel=%x want=%x", got, v)
		}
	}
	f, err := os.Open(filepath.Join(dir, "frames.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	d := json.NewDecoder(f)
	for n := uint64(40); n < 43; n++ {
		var m menuOutputFrame
		if err := d.Decode(&m); err != nil {
			t.Fatal(err)
		}
		if m.Frame != n || m.Width != 2 || m.Height != 2 || !m.HD || m.Offset != int64(n-40)*8 || m.Bytes != 8 {
			t.Fatalf("metadata=%+v", m)
		}
	}
	var extra menuOutputFrame
	if err := d.Decode(&extra); err != io.EOF {
		t.Fatalf("extra=%v", err)
	}
	assertMenuOutputStatus(t, dir, "complete", 3)
	if _, err := newMenuOutputRecorder(dir); err == nil {
		t.Fatal("overwrote existing capture")
	}
}

func assertMenuOutputStatus(t *testing.T, dir, status string, count int) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s menuOutputStatus
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	if s.Status != status || s.Written != count {
		t.Fatalf("status=%+v", s)
	}
}

func TestMenuOutputRejectsInvalidSequenceAndBuffers(t *testing.T) {
	for _, mode := range []string{"gap", "duplicate", "nil", "stride", "short", "empty", "byte-limit"} {
		t.Run(mode, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "capture")
			r, err := newMenuOutputRecorder(dir)
			if err != nil {
				t.Fatal(err)
			}
			im := noximage.NewImage16(image.Rect(0, 0, 2, 2))
			if err := r.Submit(menuOutputFrame{Frame: 9}, im); err != nil {
				t.Fatal(err)
			}
			id := uint64(10)
			switch mode {
			case "gap":
				id = 11
			case "duplicate":
				id = 9
			case "nil":
				im = nil
			case "stride":
				im.Stride = 1
			case "short":
				im.Pix = nil
			case "empty":
				im.Rect = image.Rectangle{}
			case "byte-limit":
				r.bytes = menuOutputMaxBytes - 1
			}
			if err := r.Submit(menuOutputFrame{Frame: id}, im); err == nil {
				t.Fatal("invalid frame accepted")
			}
			if err := r.Close(); err == nil {
				t.Fatal("failed capture reported success")
			}
			assertMenuOutputStatus(t, dir, "failed", 1)
		})
	}
}

func TestMenuOutputFrameLimit(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "capture")
	r, err := newMenuOutputRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	// A large test queue avoids testing disk scheduling instead of the frame cap.
	if err := r.Close(); err == nil {
		t.Fatal("empty recording succeeded")
	}
	r = newMenuOutputTestRecorder(t, dir, io.Discard)
	im := noximage.NewImage16(image.Rect(0, 0, 1, 1))
	for n := uint64(1); n <= 300; n++ {
		if err := r.Submit(menuOutputFrame{Frame: n}, im); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Submit(menuOutputFrame{Frame: 301}, im); err == nil {
		t.Fatal("frame cap not enforced")
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	assertMenuOutputStatus(t, dir, "complete", 300)
}

type menuOutputTestSink struct{ io.Writer }

func (menuOutputTestSink) Close() error { return nil }
func newMenuOutputTestRecorder(t *testing.T, dir string, w io.Writer) *menuOutputRecorder {
	t.Helper()
	r := &menuOutputRecorder{dir: dir, raw: menuOutputTestSink{w}, index: menuOutputTestSink{io.Discard}, queue: make(chan menuOutputPacket, 300), done: make(chan struct{})}
	go r.run()
	return r
}

type menuOutputBlockedWriter struct{ entered, release chan struct{} }

func (w *menuOutputBlockedWriter) Write(b []byte) (int, error) {
	select {
	case w.entered <- struct{}{}:
	default:
	}
	<-w.release
	return len(b), nil
}

type menuOutputFailedWriter struct{}

func (menuOutputFailedWriter) Write([]byte) (int, error) {
	return 0, errors.New("injected disk failure")
}

func TestMenuOutputQueueAndWriteFailures(t *testing.T) {
	t.Run("queue", func(t *testing.T) {
		dir := t.TempDir()
		w := &menuOutputBlockedWriter{make(chan struct{}, 1), make(chan struct{})}
		r := &menuOutputRecorder{dir: dir, raw: menuOutputTestSink{w}, index: menuOutputTestSink{io.Discard}, queue: make(chan menuOutputPacket, 1), done: make(chan struct{})}
		go r.run()
		im := noximage.NewImage16(image.Rect(0, 0, 1, 1))
		if err := r.Submit(menuOutputFrame{Frame: 1}, im); err != nil {
			t.Fatal(err)
		}
		<-w.entered
		if err := r.Submit(menuOutputFrame{Frame: 2}, im); err != nil {
			t.Fatal(err)
		}
		if err := r.Submit(menuOutputFrame{Frame: 3}, im); err == nil {
			t.Fatal("queue overflow hidden")
		}
		close(w.release)
		if err := r.Close(); err == nil {
			t.Fatal("queue overflow reported success")
		}
		assertMenuOutputStatus(t, dir, "failed", 2)
	})
	t.Run("write", func(t *testing.T) {
		dir := t.TempDir()
		r := newMenuOutputTestRecorder(t, dir, menuOutputFailedWriter{})
		_ = r.Submit(menuOutputFrame{Frame: 1}, noximage.NewImage16(image.Rect(0, 0, 1, 1)))
		if err := r.Close(); err == nil {
			t.Fatal("disk error hidden")
		}
		assertMenuOutputStatus(t, dir, "failed", 0)
	})
}
