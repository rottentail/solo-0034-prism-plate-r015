// Package paint implements linear-light, premultiplied-alpha compositing.
package paint

import (
	"math"
	"prismplate/internal/geom"
	"prismplate/internal/scene"
)

type Pixel struct{ R, G, B, A float64 }

func linear(v float64) float64 {
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}
func srgb(v float64) float64 {
	v = math.Max(0, math.Min(1, v))
	if v <= 0.0031308 {
		return v * 12.92
	}
	return 1.055*math.Pow(v, 1/2.4) - 0.055
}
func From(c scene.Color) Pixel {
	return Pixel{linear(c.R) * c.A, linear(c.G) * c.A, linear(c.B) * c.A, c.A}
}
func (p Pixel) Scale(a float64) Pixel { return Pixel{p.R * a, p.G * a, p.B * a, p.A * a} }

// Alpha returns the brush's own coverage at p: 1 for an opaque solid, the color
// alpha for a translucent solid, and the interpolated stop alpha for a gradient.
// Gradient RGB is ignored so a mask sample never leaks colour into the layer.
func Alpha(b scene.Brush, p geom.Point) float64 {
	if b.Kind == "solid" {
		return b.Color.A
	}
	dx, dy := b.To.X-b.From.X, b.To.Y-b.From.Y
	t := ((p.X-b.From.X)*dx + (p.Y-b.From.Y)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	for i := 1; i < len(b.Stops); i++ {
		a, z := b.Stops[i-1], b.Stops[i]
		if t <= z.At {
			u := (t - a.At) / (z.At - a.At)
			return mix(a.Color.A, z.Color.A, u)
		}
	}
	return b.Stops[len(b.Stops)-1].Color.A
}
func (p Pixel) RGBA8() (uint8, uint8, uint8, uint8) {
	if p.A <= 0 {
		return 0, 0, 0, 0
	}
	return uint8(math.Round(srgb(p.R/p.A) * 255)), uint8(math.Round(srgb(p.G/p.A) * 255)), uint8(math.Round(srgb(p.B/p.A) * 255)), uint8(math.Round(math.Min(1, p.A) * 255))
}
func mix(a, b, t float64) float64 { return a + (b-a)*t }
func Sample(b scene.Brush, p geom.Point) Pixel {
	if b.Kind == "solid" {
		return From(b.Color)
	}
	dx, dy := b.To.X-b.From.X, b.To.Y-b.From.Y
	t := ((p.X-b.From.X)*dx + (p.Y-b.From.Y)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	for i := 1; i < len(b.Stops); i++ {
		a, z := b.Stops[i-1], b.Stops[i]
		if t <= z.At {
			u := (t - a.At) / (z.At - a.At)
			x, y := From(a.Color), From(z.Color)
			return Pixel{mix(x.R, y.R, u), mix(x.G, y.G, u), mix(x.B, y.B, u), mix(x.A, y.A, u)}
		}
	}
	return From(b.Stops[len(b.Stops)-1].Color)
}
func Over(dst, src Pixel, mode string) Pixel {
	blend := func(d, s float64) float64 {
		if mode == "normal" || src.A == 0 || dst.A == 0 {
			return s + d*(1-src.A)
		}
		a, b := d/dst.A, s/src.A
		v := a * b
		if mode == "screen" {
			v = a + b - a*b
		}
		return (1-src.A)*d + (1-dst.A)*s + src.A*dst.A*v
	}
	return Pixel{blend(dst.R, src.R), blend(dst.G, src.G), blend(dst.B, src.B), src.A + dst.A*(1-src.A)}
}

// MaskCoverage combines one layer's mask shapes into a single opacity in [0,1].
// Each shape contributes its coverage, and shapes are unioned with the standard
// source-over alpha rule before the product is applied once to the layer. That
// keeps overlaps from multiplying the attenuation twice, and an empty stack is
// fully opaque rather than transparent.
func MaskCoverage(shapes []scene.CompiledMaskShape, p geom.Point) float64 {
	var coverage float64
	for _, m := range shapes {
		q := m.Inverse.Apply(p)
		if m.Path.Contains(q, m.Shape.FillRule) {
			a := Alpha(m.Shape.Brush, q)
			coverage = a + coverage*(1-a)
		}
	}
	return coverage
}
