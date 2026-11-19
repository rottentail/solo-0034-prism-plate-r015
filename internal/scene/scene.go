// Package scene defines the portable prism-plate-v1 document contract.
package scene

import "prismplate/internal/geom"

type Color struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
	A float64 `json:"a"`
}
type Stop struct {
	At    float64 `json:"at"`
	Color Color   `json:"color"`
}
type Brush struct {
	Kind  string     `json:"kind"`
	Color Color      `json:"color"`
	From  geom.Point `json:"from"`
	To    geom.Point `json:"to"`
	Stops []Stop     `json:"stops,omitempty"`
}
type Shape struct {
	ID        string      `json:"id"`
	Path      []geom.Op   `json:"path"`
	FillRule  string      `json:"fill_rule"`
	Brush     Brush       `json:"brush"`
	Transform geom.Matrix `json:"transform"`
}

// MaskShape contributes alpha to a layer's transparency mask. Only the brush
// alpha is read; RGB is retained so masks stay editable like other shapes.
type MaskShape struct {
	ID        string      `json:"id"`
	Path      []geom.Op   `json:"path"`
	FillRule  string      `json:"fill_rule"`
	Brush     Brush       `json:"brush"`
	Transform geom.Matrix `json:"transform"`
}
type Layer struct {
	ID        string      `json:"id"`
	Opacity   float64     `json:"opacity"`
	Blend     string      `json:"blend"`
	Clip      []geom.Op   `json:"clip,omitempty"`
	Mask      []geom.Op   `json:"mask,omitempty"`
	AlphaMask []MaskShape `json:"alpha_mask,omitempty"`
	Shapes    []Shape     `json:"shapes"`
}
type Document struct {
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	Background Color   `json:"background"`
	Layers     []Layer `json:"layers"`
}
type CompiledShape struct {
	Shape   Shape
	Path    geom.Path
	Inverse geom.Matrix
}
type CompiledMaskShape struct {
	Shape   MaskShape
	Path    geom.Path
	Inverse geom.Matrix
}
type CompiledLayer struct {
	Layer  Layer
	Clip   geom.Path
	Mask   geom.Path
	Alpha  []CompiledMaskShape
	Shapes []CompiledShape
}
type Compiled struct {
	Document Document
	Layers   []CompiledLayer
	Segments int
}
