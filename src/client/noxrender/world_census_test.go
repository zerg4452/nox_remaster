package noxrender

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// censusRLE builds a type-3 style record: 2 rows, width 4.
func censusRLE() []byte {
	h := make([]byte, 17)
	binary.LittleEndian.PutUint32(h[0:], 4)
	binary.LittleEndian.PutUint32(h[4:], 2)
	rows := []byte{
		1, 1, 2, 2, 0xAA, 0xAA, 0xBB, 0xBB, 4, 1, 7, // skip1, op2 x2, op4 x1
		5, 3, 1, 1, 2, 2, 3, 3, 1, 1, // op5 x3, skip1
	}
	return append(h, rows...)
}

func TestWorldCensusScanAndReport(t *testing.T) {
	c := &censusImage{Width: 4, Height: 2}
	if err := scanCensusOps(censusRLE()[17:], c); err != nil {
		t.Fatal(err)
	}
	if c.OpPix[2] != 2 || c.OpPix[4] != 1 || c.OpPix[5] != 3 || c.OpPix[1] != 0 {
		t.Fatalf("op pixels: %v", c.OpPix)
	}
	if err := scanCensusOps(censusRLE()[17:30], &censusImage{Width: 4, Height: 2}); err == nil {
		t.Fatal("truncated record accepted")
	}

	old := census
	defer func() { census = old }()
	census.path = filepath.Join(t.TempDir(), "census.json")
	census.done, census.started, census.calls, census.frame = false, time.Time{}, 0, 0
	census.perFrameCount, census.perFrameBytes = nil, nil
	var r NoxRender
	img := NewRawImage16(3, censusRLE())
	censusFrame() // no start signal yet
	r.CensusImage(img)
	if census.images != nil || !census.started.IsZero() {
		t.Fatal("collected before start signal")
	}
	if err := os.WriteFile(census.path+".start", nil, 0600); err != nil {
		t.Fatal(err)
	}
	census.calls = 0
	censusFrame() // starts
	censusFrame() // frame 1
	r.CensusImage(img)
	r.CensusImage(img) // same frame counts once
	censusFrame()      // frame 2 closes frame 1
	r.CensusImage(img)
	census.started = census.started.Add(-censusDuration)
	censusFrame() // closes frame 2 and finishes
	var rep struct {
		Frames   int            `json:"frames"`
		Unique   int            `json:"unique_images"`
		Bytes    int64          `json:"unique_hd16_bytes"`
		Mix      map[string]int `json:"op_mix"`
		PerFrame [3]int64       `json:"per_frame_images_p50_p95_max"`
		Images   []censusImage  `json:"images"`
	}
	raw, err := os.ReadFile(census.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Frames != 2 || rep.Unique != 1 || rep.Bytes != 4*2*8 || rep.Mix["has_op4_indexed"] != 1 || rep.PerFrame != [3]int64{1, 1, 1} || rep.Images[0].Frames != 2 {
		t.Fatalf("report: %+v", rep)
	}
	r.CensusImage(img)
	if census.images != nil {
		t.Fatal("census kept collecting after completion")
	}
}
