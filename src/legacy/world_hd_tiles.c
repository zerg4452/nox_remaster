#include "GAME1.h"
#include "memmap.h"

extern nox_tileDef_t nox_tile_defs_arr[176];
extern uint32_t nox_tile_def_cnt;
extern uint32_t dword_5d4594_251572;
extern obj_5D4594_2650668_t** ptr_5D4594_2650668;
extern const int ptr_5D4594_2650668_cap;

static int nox_world_hd_def_images(nox_tileDef_t* def, int count, void** out, int n, int cnt) {
	for (int i = 0; def->data_32 && i < count; i++) {
		if (def->data_32[i]) {
			if (cnt < n) {
				out[cnt] = def->data_32[i];
			}
			cnt++;
		}
	}
	return cnt;
}

// nox_world_hd_map_tile_images writes the floor images of every tile definition
// and the mask images of every edge definition used by the loaded map (all
// variations and frames) to out (4.3-001b). A cell record is {def, variation,
// ?, ?, edge list}; an edge node is {def, variation, edge def, edge image, next}.
// It returns the number of images, which exceeds n when out was truncated.
int nox_world_hd_map_tile_images(void** out, int n) {
	unsigned char tiles[176] = {0};
	unsigned char edges[64] = {0};
	if (!ptr_5D4594_2650668) {
		return 0;
	}
	for (int x = 0; x < ptr_5D4594_2650668_cap; x++) {
		for (int y = 0; y < ptr_5D4594_2650668_cap; y++) {
			obj_5D4594_2650668_t* cell = &ptr_5D4594_2650668[x][y];
			for (int k = 0; k < 2; k++) {
				if (!(cell->field_0 & (1 << k))) {
					continue;
				}
				int* rec = k ? &cell->field_6 : &cell->field_1;
				if ((unsigned int)rec[0] < 176) {
					tiles[rec[0]] = 1;
				}
				for (int* e = (int*)rec[4]; e; e = (int*)e[4]) {
					if ((unsigned int)e[0] < 176) {
						tiles[e[0]] = 1;
					}
					if ((unsigned int)e[2] < 64) {
						edges[e[2]] = 1;
					}
				}
			}
		}
	}
	int cnt = 0;
	for (int i = 0; i < 176 && i < (int)nox_tile_def_cnt; i++) {
		nox_tileDef_t* def = &nox_tile_defs_arr[i];
		if (tiles[i]) {
			cnt = nox_world_hd_def_images(def, def->field_52 * def->field_53 * def->field_54, out, n, cnt);
		}
	}
	// Edge definitions share the tile definition layout (nox_thing_read_EDGE_411850).
	nox_tileDef_t* edgeDefs = (nox_tileDef_t*)getMemAt(0x85B3FC, 28644);
	for (int i = 0; i < 64 && i < (int)dword_5d4594_251572; i++) {
		if (edges[i]) {
			cnt = nox_world_hd_def_images(&edgeDefs[i], edgeDefs[i].field_44 * edgeDefs[i].field_54, out, n, cnt);
		}
	}
	return cnt;
}
