package opennox

import (
	"github.com/noxworld-dev/opennox-lib/noximage"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestMenuOutputControlDisabledAndSignals(t *testing.T) {
	var off menuOutputCapture
	if err := off.Output(nil, false, [4]menuOutputFade{}); err != nil || off.rec != nil {
		t.Fatal("disabled capture did work")
	}
	dir := filepath.Join(t.TempDir(), "capture")
	c := menuOutputCapture{dir: dir}
	im := noximage.NewImage16(image.Rect(0, 0, 1, 1))
	if err := c.Output(im, false, [4]menuOutputFade{}); err != nil {
		t.Fatal(err)
	}
	if c.rec.submitted != 0 {
		t.Fatal("recorded before start signal")
	}
	if err := os.WriteFile(filepath.Join(dir, "start"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.Output(im, true, [4]menuOutputFade{{Remaining: 4}}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stop"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.Output(im, true, [4]menuOutputFade{}); err != nil {
		t.Fatal(err)
	}
	assertMenuOutputStatus(t, dir, "complete", 1)
	if err := c.Output(nil, false, [4]menuOutputFade{}); err != nil {
		t.Fatal(err)
	}
}

func TestMenuOutputControlErrorsOnlyOnce(t *testing.T) {
	c := menuOutputCapture{dir: t.TempDir()}
	if err := c.Output(nil, false, [4]menuOutputFade{}); err == nil {
		t.Fatal("existing directory accepted")
	}
	if err := c.Output(nil, false, [4]menuOutputFade{}); err != nil {
		t.Fatal("initialization error repeated")
	}
}

func TestMenuOutputBadStopCannotReportComplete(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "capture")
	c := menuOutputCapture{dir: dir}
	im := noximage.NewImage16(image.Rect(0, 0, 1, 1))
	if err := c.Output(im, false, [4]menuOutputFade{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "start"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := c.Output(im, false, [4]menuOutputFade{}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "stop"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := c.Output(im, false, [4]menuOutputFade{}); err == nil {
		t.Fatal("invalid stop ignored")
	}
	assertMenuOutputStatus(t, dir, "failed", 1)
}
