package noxrender

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"sync"

	"github.com/noxworld-dev/opennox-lib/noximage/pcx"

	"github.com/noxworld-dev/opennox/v1/client/noxrender/worldhd"
)

// Asynchronous world HD asset loading (4.2 design D5). The render thread
// requests assets and installs finished ones at frame start; workers only read
// PNGs from the asset root and convert them into C buffers. Workers share no
// mutable state with the render thread except the two channels. In-flight
// buffers are bounded by the job count and byte limits, and every buffer is
// either installed in the cache or freed.
const (
	hdLoaderWorkers  = 2
	hdLoaderQueue    = 64
	hdLoaderInflight = 32 << 20
)

type hdAssetJob struct {
	img  *Image
	spec worldhd.FloorSpec
	raw  []byte // original bag record, read-only
}

type hdAssetResult struct {
	img   *Image
	asset *hdAsset
	err   error
}

type hdLoaderStats struct {
	Requested, Dropped, Failed, Prefetched int64
}

type hdAssetLoader struct {
	root  fs.FS
	specs map[int]worldhd.FloorSpec
	cache *hdAssetCache
	alloc func(n int) ([]uint16, func(), bool)

	jobs    chan hdAssetJob
	results chan hdAssetResult
	wg      sync.WaitGroup
	closed  bool

	maxPending, maxInflight int
	pending                 map[*Image]int // reserved bytes
	inflight                int
	rejected                map[*Image]struct{}
	stats                   hdLoaderStats

	// prefetch holds wanted images (4.3-001a) that BeginFrame feeds to Request
	// as the job and byte limits allow, so a large prefetch is never dropped.
	prefetch []*Image
	queued   map[*Image]struct{}
}

func newHDAssetLoader(root fs.FS, specs map[int]worldhd.FloorSpec, cache *hdAssetCache) *hdAssetLoader {
	l := &hdAssetLoader{
		root: root, specs: specs, cache: cache, alloc: allocHDPixels,
		jobs:       make(chan hdAssetJob, hdLoaderQueue),
		results:    make(chan hdAssetResult, hdLoaderQueue),
		maxPending: hdLoaderQueue, maxInflight: hdLoaderInflight,
		pending: make(map[*Image]int), rejected: make(map[*Image]struct{}),
		queued: make(map[*Image]struct{}),
	}
	for i := 0; i < hdLoaderWorkers; i++ {
		l.wg.Add(1)
		go l.worker()
	}
	return l
}

func assetBytes(s worldhd.FloorSpec) int { return 2 * worldhd.AssetPixels(s) }

// Request queues img if it has a replacement that is not installed, pending or
// rejected. When limits are reached the request is dropped and retried on a
// later draw. Render thread only.
func (l *hdAssetLoader) Request(img *Image) {
	if l == nil || l.closed || img == nil || img.bag == nil {
		return
	}
	spec, ok := l.specs[img.bag.Index]
	if !ok || l.cache.byImage[img] != nil {
		return
	}
	if _, busy := l.pending[img]; busy {
		return
	}
	if _, bad := l.rejected[img]; bad {
		return
	}
	n := assetBytes(spec)
	if len(l.pending) >= l.maxPending || l.inflight+n > l.maxInflight {
		l.stats.Dropped++
		return
	}
	// Read the bag record without caching it in img.raw: a cached raw would
	// disable the image's override data (loadOverride) and change what is drawn.
	raw := img.raw
	if raw == nil {
		var err error
		if raw, err = img.bag.Raw(); err != nil {
			l.rejected[img] = struct{}{}
			l.stats.Failed++
			return
		}
	}
	select {
	case l.jobs <- hdAssetJob{img: img, spec: spec, raw: raw}:
		l.pending[img] = n
		l.inflight += n
		l.stats.Requested++
	default:
		l.stats.Dropped++
	}
}

// idle reports whether img needs no loading work: no replacement, already
// installed, converting or rejected.
func (l *hdAssetLoader) idle(img *Image) bool {
	if _, ok := l.specs[img.bag.Index]; !ok || l.cache.byImage[img] != nil {
		return true
	}
	if _, busy := l.pending[img]; busy {
		return true
	}
	_, bad := l.rejected[img]
	return bad
}

// Prefetch queues images to load ahead of drawing, in order, and returns how
// many were queued. Images without a replacement or already loaded,
// converting, rejected or queued are skipped. Render thread only.
func (l *hdAssetLoader) Prefetch(imgs []*Image) int {
	if l == nil || l.closed {
		return 0
	}
	added := 0
	for _, img := range imgs {
		if img == nil || img.bag == nil || l.idle(img) {
			continue
		}
		if _, ok := l.queued[img]; ok {
			continue
		}
		l.queued[img] = struct{}{}
		l.prefetch = append(l.prefetch, img)
		l.stats.Prefetched++
		added++
	}
	l.feedPrefetch() // start converting now, e.g. during a map load
	return added
}

// PrefetchQueued is the number of prefetched images not yet requested.
func (l *hdAssetLoader) PrefetchQueued() int {
	if l == nil {
		return 0
	}
	return len(l.prefetch)
}

// feedPrefetch requests queued images until the next one does not fit the
// limits. An asset larger than the byte limit can never load and is dropped.
func (l *hdAssetLoader) feedPrefetch() {
	n := 0
	for ; n < len(l.prefetch); n++ {
		img := l.prefetch[n]
		if !l.idle(img) {
			size := assetBytes(l.specs[img.bag.Index])
			if size <= l.maxInflight && (len(l.pending) >= l.maxPending || l.inflight+size > l.maxInflight) {
				break
			}
			l.Request(img)
		}
		delete(l.queued, img)
	}
	l.prefetch = append(l.prefetch[:0], l.prefetch[n:]...)
}

// BeginFrame starts a cache frame, installs finished assets and feeds queued
// prefetches. Render thread only.
func (l *hdAssetLoader) BeginFrame() {
	if l == nil {
		return
	}
	l.cache.BeginFrame()
	for {
		select {
		case r := <-l.results:
			l.finish(r)
		default:
			if !l.closed {
				l.feedPrefetch()
			}
			return
		}
	}
}

func (l *hdAssetLoader) finish(r hdAssetResult) {
	l.inflight -= l.pending[r.img]
	delete(l.pending, r.img)
	if r.err != nil {
		l.rejected[r.img] = struct{}{}
		l.stats.Failed++
		if l.stats.Failed <= 20 {
			Log.Printf("world-hd asset rejected; original retained: %v", r.err)
		}
		return
	}
	if l.closed {
		r.asset.free()
		return
	}
	l.cache.Install(r.asset)
}

// Close stops the workers, frees every buffer still in flight and clears the
// cache. Call when the bag is replaced or world HD assets are disabled.
func (l *hdAssetLoader) Close() {
	if l == nil || l.closed {
		return
	}
	l.closed = true
	l.prefetch, l.queued = nil, nil
	close(l.jobs)
	l.wg.Wait()
	for len(l.results) > 0 {
		l.finish(<-l.results)
	}
	l.cache.Clear()
}

func (l *hdAssetLoader) worker() {
	defer l.wg.Done()
	for job := range l.jobs {
		l.results <- l.convert(job)
	}
}

func (l *hdAssetLoader) convert(job hdAssetJob) hdAssetResult {
	res := hdAssetResult{img: job.img}
	s := job.spec
	src := worldhd.FloorSource{Type: s.Type, Raw: job.raw}
	if s.Type == 0 {
		decoded, err := pcx.Decode(bytes.NewReader(job.raw), byte(s.Type))
		if err != nil {
			res.err = err
			return res
		}
		src.Mask, src.Offset = decoded.Image, decoded.Point
	}
	f, err := l.root.Open(s.Path)
	if err != nil {
		res.err = err
		return res
	}
	data, err := io.ReadAll(io.LimitReader(f, worldhd.MaxAssetPNG+1))
	if ce := f.Close(); err == nil {
		err = ce
	}
	if err != nil {
		res.err = err
		return res
	}
	pix, free, ok := l.alloc(worldhd.AssetPixels(s))
	if !ok {
		res.err = fmt.Errorf("asset %d: C allocation failed", s.ID)
		return res
	}
	if err = worldhd.ConvertAsset(s, data, src, pix); err != nil {
		free()
		res.err = err
		return res
	}
	res.asset = &hdAsset{img: job.img, spec: s, pix: pix, free: free, bytes: assetBytes(s)}
	return res
}
