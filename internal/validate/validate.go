// Package validate owns structural admission and the shared compilation budget.
package validate

import (
	"fmt"
	"math"
	"prismplate/internal/geom"
	"prismplate/internal/scene"
	"regexp"
)

var identifier = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,63}$`)

func ID(s string) bool         { return identifier.MatchString(s) }
func finite(x float64) bool    { return !math.IsNaN(x) && !math.IsInf(x, 0) && math.Abs(x) <= 1e6 }
func Unit(x float64) bool      { return finite(x) && x >= 0 && x <= 1 }
func Color(c scene.Color) bool { return Unit(c.R) && Unit(c.G) && Unit(c.B) && Unit(c.A) }
func path(ops []geom.Op) (geom.Path, error) {
	if len(ops) == 0 || len(ops) > 256 {
		return nil, fmt.Errorf("contour operation count must be 1..256")
	}
	open := false
	vertices := 0
	for _, o := range ops {
		n, ok := map[string]int{"M": 1, "L": 1, "Q": 2, "C": 3, "Z": 0}[o.Kind]
		if !ok || len(o.Points) != n {
			return nil, fmt.Errorf("invalid contour verb or arity")
		}
		for _, p := range o.Points {
			if !finite(p.X) || !finite(p.Y) {
				return nil, fmt.Errorf("non-finite contour")
			}
		}
		if o.Kind == "M" {
			if open {
				return nil, fmt.Errorf("close each contour before another M")
			}
			open = true
			vertices = 1
		} else {
			if !open {
				return nil, fmt.Errorf("contour must begin with M")
			}
			if o.Kind == "Z" {
				if vertices < 3 {
					return nil, fmt.Errorf("contour requires three vertices")
				}
				open = false
			} else {
				vertices++
			}
		}
	}
	if open {
		return nil, fmt.Errorf("contour must terminate with Z")
	}
	return geom.Flatten(ops), nil
}
func brush(b scene.Brush) error {
	if b.Kind == "solid" {
		if !Color(b.Color) {
			return fmt.Errorf("invalid solid color")
		}
		return nil
	}
	if b.Kind != "linear" {
		return fmt.Errorf("unknown brush")
	}
	for _, v := range []float64{b.From.X, b.From.Y, b.To.X, b.To.Y} {
		if !finite(v) {
			return fmt.Errorf("invalid gradient axis")
		}
	}
	if math.Hypot(b.To.X-b.From.X, b.To.Y-b.From.Y) < 1e-6 {
		return fmt.Errorf("degenerate gradient axis")
	}
	if len(b.Stops) < 2 || len(b.Stops) > 16 {
		return fmt.Errorf("gradient requires 2..16 stops")
	}
	prev := -1.0
	for _, s := range b.Stops {
		if !Unit(s.At) || s.At <= prev || !Color(s.Color) {
			return fmt.Errorf("invalid gradient stops")
		}
		prev = s.At
	}
	if b.Stops[0].At != 0 || prev != 1 {
		return fmt.Errorf("gradient endpoints must be 0 and 1")
	}
	return nil
}
func transform(m geom.Matrix) (geom.Matrix, error) {
	for _, v := range m {
		if !finite(v) {
			return geom.Matrix{}, fmt.Errorf("invalid transform")
		}
	}
	inv, ok := m.Inverse()
	if !ok {
		return geom.Matrix{}, fmt.Errorf("singular transform")
	}
	for _, v := range inv {
		if !finite(v) {
			return geom.Matrix{}, fmt.Errorf("ill-conditioned transform")
		}
	}
	return inv, nil
}
func alphaMask(ms []scene.MaskShape, add func(string) error) ([]scene.CompiledMaskShape, int, error) {
	out := make([]scene.CompiledMaskShape, 0, len(ms))
	segments := 0
	for _, m := range ms {
		if err := add(m.ID); err != nil {
			return nil, 0, err
		}
		if m.FillRule != "nonzero" && m.FillRule != "evenodd" {
			return nil, 0, fmt.Errorf("invalid mask fill rule")
		}
		inv, err := transform(m.Transform)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid mask transform: %w", err)
		}
		p, err := path(m.Path)
		if err != nil {
			return nil, 0, err
		}
		if err = brush(m.Brush); err != nil {
			return nil, 0, fmt.Errorf("invalid mask brush: %w", err)
		}
		for _, r := range p {
			segments += len(r)
		}
		out = append(out, scene.CompiledMaskShape{Shape: m, Path: p, Inverse: inv})
	}
	return out, segments, nil
}
func Compile(d scene.Document) (scene.Compiled, error) {
	c := scene.Compiled{Document: d}
	if d.Width < 1 || d.Height < 1 || d.Width > 512 || d.Height > 512 || !Color(d.Background) {
		return c, fmt.Errorf("invalid canvas (maximum 512 square)")
	}
	if len(d.Layers) < 1 || len(d.Layers) > 16 {
		return c, fmt.Errorf("requires 1..16 layers")
	}
	ids := map[string]bool{}
	add := func(id string) error {
		if !ID(id) || ids[id] {
			return fmt.Errorf("invalid or duplicate node id %q", id)
		}
		ids[id] = true
		return nil
	}
	count := func(p geom.Path) {
		for _, r := range p {
			c.Segments += len(r)
		}
	}
	for _, l := range d.Layers {
		if err := add(l.ID); err != nil {
			return c, err
		}
		if !Unit(l.Opacity) || (l.Blend != "normal" && l.Blend != "multiply" && l.Blend != "screen") {
			return c, fmt.Errorf("invalid layer compositing")
		}
		if len(l.AlphaMask) > 16 {
			return c, fmt.Errorf("requires at most 16 alpha mask shapes per layer")
		}
		if len(l.Shapes) < 1 || len(l.Shapes) > 32 {
			return c, fmt.Errorf("requires 1..32 shapes per layer")
		}
		cl := scene.CompiledLayer{Layer: l}
		if len(l.Clip) > 0 {
			p, e := path(l.Clip)
			if e != nil {
				return c, e
			}
			cl.Clip = p
			count(p)
		}
		if len(l.Mask) > 0 {
			p, e := path(l.Mask)
			if e != nil {
				return c, e
			}
			cl.Mask = p
			count(p)
		}
		if len(l.AlphaMask) > 0 {
			am, segs, e := alphaMask(l.AlphaMask, add)
			if e != nil {
				return c, e
			}
			cl.Alpha = am
			c.Segments += segs
		}
		for _, s := range l.Shapes {
			if err := add(s.ID); err != nil {
				return c, err
			}
			if s.FillRule != "nonzero" && s.FillRule != "evenodd" {
				return c, fmt.Errorf("invalid fill rule")
			}
			for _, v := range s.Transform {
				if !finite(v) {
					return c, fmt.Errorf("invalid transform")
				}
			}
			inv, ok := s.Transform.Inverse()
			if !ok {
				return c, fmt.Errorf("singular transform")
			}
			for _, v := range inv {
				if !finite(v) {
					return c, fmt.Errorf("ill-conditioned transform")
				}
			}
			p, e := path(s.Path)
			if e != nil {
				return c, e
			}
			if e = brush(s.Brush); e != nil {
				return c, e
			}
			count(p)
			cl.Shapes = append(cl.Shapes, scene.CompiledShape{Shape: s, Path: p, Inverse: inv})
		}
		c.Layers = append(c.Layers, cl)
	}
	// Four samples per pixel; every fill is bounded even under adversarial input.
	if int64(c.Segments)*int64(d.Width)*int64(d.Height)*4 > 160_000_000 {
		return c, fmt.Errorf("scene exceeds bounded rendering work")
	}
	return c, nil
}
