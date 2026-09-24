package opennox

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/noxworld-dev/opennox-lib/noximage"
)

const menuOutputMaxBytes int64 = 2 << 30
const menuOutputMaxFrames = 300

type menuOutputFade struct {
	Key       int `json:"key"`
	Flags     int `json:"flags"`
	Remaining int `json:"remaining"`
}
type menuOutputFrame struct {
	Frame  uint64            `json:"frame"`
	Width  int               `json:"width"`
	Height int               `json:"height"`
	HD     bool              `json:"hd"`
	Fade   [4]menuOutputFade `json:"fade_post_draw"`
	Offset int64             `json:"offset"`
	Bytes  int64             `json:"bytes"`
}
type menuOutputStatus struct {
	Status    string `json:"status"`
	Submitted int    `json:"submitted"`
	Written   int    `json:"written"`
	Error     string `json:"error,omitempty"`
}
type menuOutputPacket struct{ pixels, metadata []byte }
type menuOutputRecorder struct {
	dir                string
	raw, index         io.WriteCloser
	queue              chan menuOutputPacket
	done               chan struct{}
	mu                 sync.Mutex
	err                error
	closed             bool
	submitted, written int
	last               uint64
	bytes, rawBytes    int64
}

func newMenuOutputRecorder(dir string) (*menuOutputRecorder, error) {
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, err
	}
	r := &menuOutputRecorder{dir: dir, queue: make(chan menuOutputPacket, 4), done: make(chan struct{})}
	if err := r.writeStatus(menuOutputStatus{Status: "recording"}); err != nil {
		return nil, err
	}
	var err error
	r.raw, err = os.OpenFile(filepath.Join(dir, "frames.bin"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		r.index, err = os.OpenFile(filepath.Join(dir, "frames.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	}
	if err != nil {
		if r.raw != nil {
			_ = r.raw.Close()
		}
		_ = r.writeStatus(menuOutputStatus{Status: "failed", Error: err.Error()})
		return nil, err
	}
	go r.run()
	return r, nil
}

// Submit copies the selected output. No borrowed render pixels reach the worker.
// A rejected frame fails this capture, except submissions after its normal cap.
func (r *menuOutputRecorder) Submit(m menuOutputFrame, im *noximage.Image16) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	if r.closed {
		return errors.New("output recording is closed")
	}
	bad := func(err error) error { r.err = err; r.closed = true; close(r.queue); return err }
	if r.submitted > 0 && (r.last == ^uint64(0) || m.Frame != r.last+1) {
		return bad(errors.New("output frame sequence is discontinuous"))
	}
	if im == nil || im.Rect.Empty() {
		return bad(errors.New("empty output frame"))
	}
	w, h := im.Rect.Dx(), im.Rect.Dy()
	if im.Stride < w || im.Stride <= 0 || int64(h-1)*int64(im.Stride)+int64(w) > int64(len(im.Pix)) {
		return bad(errors.New("invalid output pixel storage"))
	}
	m.Width, m.Height = w, h
	m.Offset = r.rawBytes
	m.Bytes = int64(w) * int64(h) * 2
	meta, err := json.Marshal(m)
	if err != nil {
		return bad(err)
	}
	meta = append(meta, '\n')
	// Reserve 4KiB for status and control files. Reject before allocating pixels.
	if m.Bytes <= 0 || m.Bytes > menuOutputMaxBytes-4096-r.bytes-int64(len(meta)) {
		return bad(errors.New("output recording exceeds 2GiB bound"))
	}
	pix := make([]byte, int(m.Bytes))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			binary.LittleEndian.PutUint16(pix[(y*w+x)*2:], im.Pix[y*im.Stride+x])
		}
	}
	select {
	case r.queue <- menuOutputPacket{pix, meta}:
		r.submitted++
		r.last = m.Frame
		r.rawBytes += m.Bytes
		r.bytes += m.Bytes + int64(len(meta))
		if r.submitted == menuOutputMaxFrames {
			r.closed = true
			close(r.queue)
		}
		return nil
	default:
		return bad(errors.New("output recording queue overflow; capture failed"))
	}
}

func (r *menuOutputRecorder) Close() error {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.queue)
	}
	if r.submitted == 0 && r.err == nil {
		r.err = errors.New("no output frames recorded")
	}
	r.mu.Unlock()
	<-r.done
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

func (r *menuOutputRecorder) writeStatus(s menuOutputStatus) error {
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.dir, "status.json"), append(b, '\n'), 0600)
}
func menuOutputWrite(w io.Writer, b []byte) error {
	n, err := w.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	return err
}
func (r *menuOutputRecorder) run() {
	defer close(r.done)
	var writeErr error
	for p := range r.queue {
		if writeErr != nil {
			continue
		}
		writeErr = menuOutputWrite(r.raw, p.pixels)
		if writeErr == nil {
			writeErr = menuOutputWrite(r.index, p.metadata)
		}
		if writeErr == nil {
			r.written++
		} else {
			r.mu.Lock()
			if r.err == nil {
				r.err = writeErr
			}
			r.mu.Unlock()
		}
	}
	for _, f := range []io.WriteCloser{r.raw, r.index} {
		if err := f.Close(); err != nil && writeErr == nil {
			writeErr = err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err == nil {
		r.err = writeErr
	}
	if r.err == nil && r.submitted != r.written {
		r.err = fmt.Errorf("output frames missing: submitted=%d written=%d", r.submitted, r.written)
	}
	s := menuOutputStatus{Status: "complete", Submitted: r.submitted, Written: r.written}
	if r.err != nil {
		s.Status = "failed"
		s.Error = r.err.Error()
	}
	if err := r.writeStatus(s); err != nil && r.err == nil {
		r.err = err
	}
}
