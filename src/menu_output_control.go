package opennox

import (
	"errors"
	"github.com/noxworld-dev/opennox-lib/noximage"
	"os"
	"path/filepath"
)

// One opt-in capture per process; existing recordings are never overwritten.
type menuOutputCapture struct {
	dir             string
	rec             *menuOutputRecorder
	armed, finished bool
	frame           uint64
}

func menuOutputSignal(path string) (bool, error) {
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !st.Mode().IsRegular() || st.Size() != 0 {
		return false, errors.New("output capture signal must be an empty regular file")
	}
	return true, nil
}

func (c *menuOutputCapture) Output(im *noximage.Image16, hd bool, fade [4]menuOutputFade) error {
	if c.dir == "" || c.finished {
		return nil
	}
	c.frame++
	if c.rec == nil {
		if !filepath.IsAbs(c.dir) {
			c.finished = true
			return errors.New("output capture requires an absolute new directory")
		}
		var err error
		c.rec, err = newMenuOutputRecorder(c.dir)
		if err != nil {
			c.finished = true
			return err
		}
	}
	if !c.armed {
		start, err := menuOutputSignal(filepath.Join(c.dir, "start"))
		if err != nil {
			return c.fail(err)
		}
		if !start {
			return nil
		}
		c.armed = true
	}
	stop, err := menuOutputSignal(filepath.Join(c.dir, "stop"))
	if err != nil {
		return c.fail(err)
	}
	if stop {
		return c.Close()
	}
	if err := c.rec.Submit(menuOutputFrame{Frame: c.frame, HD: hd, Fade: fade}, im); err != nil {
		_ = c.Close()
		return err
	}
	if c.rec.submitted == menuOutputMaxFrames {
		return c.Close()
	}
	return nil
}

func (c *menuOutputCapture) fail(err error) error {
	c.rec.mu.Lock()
	if c.rec.err == nil {
		c.rec.err = err
	}
	c.rec.mu.Unlock()
	_ = c.Close()
	return err
}

func (c *menuOutputCapture) Close() error {
	if c.finished {
		return nil
	}
	c.finished = true
	if c.rec != nil {
		return c.rec.Close()
	}
	return nil
}
