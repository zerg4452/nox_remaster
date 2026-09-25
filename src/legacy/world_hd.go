package legacy

/*
extern unsigned int nox_world_hd_tiles;
int nox_world_hd_map_tile_images(void** out, int n);
*/
import "C"
import "unsafe"

var WorldHDTile func(int, int, unsafe.Pointer, unsafe.Pointer)
var WorldHDUnsupported func(int)
var WorldHDTileOpaque func(int, []uint16)

//export nox_world_hd_tile_opaque
func nox_world_hd_tile_opaque(index C.int, src *C.ushort, n C.int) {
	if WorldHDTileOpaque != nil && n > 0 {
		WorldHDTileOpaque(int(index), unsafe.Slice((*uint16)(unsafe.Pointer(src)), int(n)))
	}
}

func SetWorldHDTiles(enabled bool) {
	if enabled {
		C.nox_world_hd_tiles = 1
	} else {
		C.nox_world_hd_tiles = 0
	}
}

//export nox_world_hd_tile
func nox_world_hd_tile(x, y C.int, base, edge unsafe.Pointer) {
	if WorldHDTile != nil {
		WorldHDTile(int(x), int(y), base, edge)
	}
}

// WorldHDWallBegin and WorldHDWallSpan receive the lit opaque wall image and
// the image-local position of each lit span (4.3-001c).
var (
	WorldHDWallBegin func(unsafe.Pointer)
	WorldHDWallSpan  func(x, y int)
)

//export nox_world_hd_wall_begin
func nox_world_hd_wall_begin(img unsafe.Pointer) {
	if WorldHDWallBegin != nil {
		WorldHDWallBegin(img)
	}
}

//export nox_world_hd_wall_span
func nox_world_hd_wall_span(x, y C.int) {
	if WorldHDWallSpan != nil {
		WorldHDWallSpan(int(x), int(y))
	}
}

//export nox_world_hd_unsupported
func nox_world_hd_unsupported(kind C.int) {
	if WorldHDUnsupported != nil {
		WorldHDUnsupported(int(kind))
	}
}

// WorldHDMapTileImages returns the floor and edge image handles of every tile
// and edge definition used by the loaded map (4.3-001b).
func WorldHDMapTileImages() []unsafe.Pointer {
	n := int(C.nox_world_hd_map_tile_images(nil, 0))
	if n <= 0 {
		return nil
	}
	buf := make([]uintptr, n)
	n = min(n, int(C.nox_world_hd_map_tile_images((*unsafe.Pointer)(unsafe.Pointer(&buf[0])), C.int(n))))
	out := make([]unsafe.Pointer, n)
	for i := range out {
		out[i] = unsafe.Pointer(buf[i]) // C memory, not managed by Go
	}
	return out
}
