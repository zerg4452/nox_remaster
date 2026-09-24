// Package worldhd validates optional world assets independently of menu assets.
// It does not enable HD presentation or change legacy tile pixels.
package worldhd

import "image"

// FloorSpec binds a replacement to the raw bag record, not its non-unique name
// or an exported PNG's hash. Despite the name it describes any supported
// record type (see ConvertAsset); density is 2.
type FloorSpec struct {
	ID           int
	Type         int
	SourceSHA256 [32]byte
	PNGSHA256    [32]byte
	Path         string
	LogicalSize  image.Point
	Offset       image.Point
	Density      int
}

// FloorSource must be resolved from the current bag by record index. Raw is the
// unmodified record payload (before overrides); Mask and Offset are the decoded
// original and are required for type 0 floors only.
type FloorSource struct {
	Type   int
	Raw    []byte
	Mask   image.Image
	Offset image.Point
}
