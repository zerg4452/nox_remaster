package legacy

/*
extern unsigned int nox_world_hd_tiles;
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

//export nox_world_hd_unsupported
func nox_world_hd_unsupported(kind C.int) {
	if WorldHDUnsupported != nil {
		WorldHDUnsupported(int(kind))
	}
}
