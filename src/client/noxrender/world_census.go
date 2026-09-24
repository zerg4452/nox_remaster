package noxrender

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// World image census (4.2-M1 diagnostic). NOX_WORLD_IMAGE_CENSUS names an
// absolute report path; collection starts when "<path>.start" exists and covers
// censusDuration of world HD frames, then one JSON report is written. It only
// observes draws and never changes rendering.
const censusDuration = 60 * time.Second

type censusImage struct {
	Index  int     `json:"index"` // bag record, -1 for non-bag images
	Type   int     `json:"type"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
	OpPix  [16]int `json:"op_pixels"` // opaque pixels per RLE op (op&0xF)
	Frames int     `json:"frames"`
	Error  string  `json:"scan_error,omitempty"`
	last   int
}

var census = struct {
	path          string
	started       time.Time
	done          bool
	calls, frame  int
	images        map[Image16]*censusImage
	frameImages   int
	frameBytes    int64
	perFrameCount []int
	perFrameBytes []int64
	pixStart      [2]int64
}{path: os.Getenv("NOX_WORLD_IMAGE_CENSUS")}

// pixdataInterned counts original image data copied to C memory by Pixdata;
// that memory is only released when the bag is freed.
var pixdataInterned struct{ bytes, count int64 }

func censusActive() bool { return census.path != "" && !census.done && !census.started.IsZero() }

// censusFrame closes the previous world frame and starts or finishes collection.
func censusFrame() {
	if census.path == "" || census.done {
		return
	}
	if census.started.IsZero() {
		if census.calls++; census.calls%30 != 1 {
			return
		}
		if !filepath.IsAbs(census.path) {
			census.done = true
			Log.Println("world-census rejected relative path")
			return
		}
		if st, err := os.Lstat(census.path + ".start"); err != nil || !st.Mode().IsRegular() {
			return
		}
		census.started = time.Now()
		census.images = make(map[Image16]*censusImage)
		census.pixStart = [2]int64{pixdataInterned.bytes, pixdataInterned.count}
		return
	}
	if census.frame > 0 {
		census.perFrameCount = append(census.perFrameCount, census.frameImages)
		census.perFrameBytes = append(census.perFrameBytes, census.frameBytes)
	}
	census.frame++
	census.frameImages, census.frameBytes = 0, 0
	if time.Since(census.started) >= censusDuration {
		finishCensus()
	}
}

// CensusImage records one image drawn in the current world frame.
func (r *NoxRender) CensusImage(img Image16) {
	if !censusActive() || census.frame == 0 || img == nil {
		return
	}
	c := census.images[img]
	if c == nil {
		c = scanCensusImage(img)
		census.images[img] = c
	}
	if c.last != census.frame {
		c.last = census.frame
		c.Frames++
		census.frameImages++
		census.frameBytes += int64(c.Width*c.Height) * 8 // density 2, 16-bit
	}
}

func scanCensusImage(img Image16) *censusImage {
	c := &censusImage{Index: -1, Type: img.Type() & 0x3F}
	if p, ok := img.(*Image); ok && p.bag != nil {
		c.Index = int(p.bag.Index)
	}
	switch c.Type {
	case 0, 1:
		c.Width, c.Height = 46, 46
		return c
	case 3, 4, 5, 6, 8:
	default:
		return c
	}
	pix := img.Pixdata()
	if len(pix) < 17 {
		c.Error = "short header"
		return c
	}
	c.Width = int(binary.LittleEndian.Uint32(pix[0:]))
	c.Height = int(binary.LittleEndian.Uint32(pix[4:]))
	if err := scanCensusOps(pix[17:], c); err != nil {
		c.Error = err.Error()
	}
	return c
}

func scanCensusOps(pix []byte, c *censusImage) error {
	for y := 0; y < c.Height; y++ {
		for x := 0; x < c.Width; {
			if len(pix) < 2 {
				return errors.New("truncated run")
			}
			op, n := pix[0]&0xF, int(pix[1])
			pix = pix[2:]
			size := 0
			switch op {
			case 1:
			case 2, 5, 6, 7:
				size = 2 * n
			case 4:
				size = n
			default:
				return errors.New("invalid op")
			}
			if len(pix) < size || n == 0 {
				return errors.New("invalid run length")
			}
			pix = pix[size:]
			if op != 1 {
				c.OpPix[op] += n
			}
			x += n
		}
	}
	return nil
}

func percentiles(v []int64) [3]int64 {
	if len(v) == 0 {
		return [3]int64{}
	}
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	at := func(p float64) int64 { return s[int(float64(len(s))*p+0.999999)-1] }
	return [3]int64{at(0.50), at(0.95), s[len(s)-1]}
}

func finishCensus() {
	census.done = true
	counts := make([]int64, len(census.perFrameCount))
	for i, v := range census.perFrameCount {
		counts[i] = int64(v)
	}
	type typeSum struct {
		Images int   `json:"images"`
		Bytes  int64 `json:"hd16_bytes"`
	}
	byType := map[int]*typeSum{}
	var list []*censusImage
	var total int64
	mix := map[string]int{}
	for _, c := range census.images {
		list = append(list, c)
		b := int64(c.Width*c.Height) * 8
		total += b
		t := byType[c.Type]
		if t == nil {
			t = &typeSum{}
			byType[c.Type] = t
		}
		t.Images++
		t.Bytes += b
		switch {
		case c.Type < 3 || c.Type > 8 || c.Type == 7:
			mix["not_rle"]++
		case c.Error != "":
			mix["scan_error"]++
		case c.OpPix[4] > 0:
			mix["has_op4_indexed"]++
		case c.OpPix[6] > 0:
			mix["has_op6"]++
		default:
			mix["only_op2_5_7"]++
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Frames > list[j].Frames })
	report := map[string]any{
		"status":                           "complete",
		"duration_ns":                      int64(time.Since(census.started)),
		"frames":                           len(census.perFrameCount),
		"per_frame_images_p50_p95_max":     percentiles(counts),
		"per_frame_hd16_bytes_p50_p95_max": percentiles(census.perFrameBytes),
		"unique_images":                    len(list),
		"unique_hd16_bytes":                total,
		"by_type":                          byType,
		"op_mix":                           mix,
		"pixdata_interned_start":           census.pixStart,
		"pixdata_interned_end":             [2]int64{pixdataInterned.bytes, pixdataInterned.count},
		"note":                             "hd16 bytes = width*height*4*2 upper bound per image; bytes/count pairs for pixdata",
		"images":                           list,
	}
	census.images = nil
	b, err := json.MarshalIndent(report, "", " ")
	if err == nil {
		var f *os.File
		if f, err = os.OpenFile(census.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600); err == nil {
			_, err = f.Write(b)
			if ce := f.Close(); err == nil {
				err = ce
			}
		}
	}
	Log.Printf("world-census frames=%d unique=%d hd16_bytes=%d error=%v", len(census.perFrameCount), len(list), total, err)
}
