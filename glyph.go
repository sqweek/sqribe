package main

import (
	"image/color"
	"image/draw"
	"image"
	"math"
)

// A Glyph is an Image with (0, 0) treated as its "hotspot"
type Glyph image.Image

// Paints a Glyph with its (0, 0) coordinate aligned with pt
func DrawGlyph(dst draw.Image, r image.Rectangle, glyph Glyph, col color.Color, pt image.Point) {
	draw.DrawMask(dst, r, &image.Uniform{col}, image.ZP, glyph, r.Min.Sub(pt), draw.Over)
}

type GlyphPalette struct {
	SolidNote,
	HollowNote,
	DownTail,
	UpTail,

	Flat,
	Sharp,
	Natural,
	Placeholder,

	Dot,

	TrebleClef,
	BassClef Glyph
}
var Glyphs GlyphPalette

func (p *GlyphPalette) init(sz int) {
	r :=  sz/2
	cent := CenteredGlyph{r: r}
	p.SolidNote = &NoteHead{cent, 0.0}
	p.HollowNote = &NoteHead{cent, 0.6}
	tailsz := 4*sz/(2*5)
	p.DownTail = &NoteTail{sz: tailsz, downBeam: false}
	p.UpTail = &NoteTail{sz: tailsz, downBeam: true}

	p.Flat = &FlatGlyph{r: r}
	p.Sharp = &SharpGlyph{cent}
	p.Natural = &NaturalGlyph{cent}
	p.Placeholder = &DefaultGlyph{cent}

	p.Dot = &DotGlyph{cent}
}

func (p GlyphPalette) SharpOrFlat(sharp bool) Glyph {
	if sharp {
		return p.Sharp
	}
	return p.Flat
}

func (p GlyphPalette) NoteHead(solid bool) Glyph {
	if solid {
		return p.SolidNote
	}
	return p.HollowNote
}

func (p GlyphPalette) NoteTail(downBeam bool) Glyph {
	if downBeam {
		return p.UpTail
	}
	return p.DownTail
}

func (p GlyphPalette) Ax(accidental int) Glyph {
	switch accidental {
	case -1: return Glyphs.Flat
	case 0: return Glyphs.Natural
	case 1: return Glyphs.Sharp
	}
	return Glyphs.Placeholder
}

type glyphBase struct {}
func (g *glyphBase) ColorModel() color.Model {
	return color.AlphaModel
}

type CenteredGlyph struct {
	glyphBase
	r int //radius, (0, 0) is the centre point
}

func (g *CenteredGlyph) Bounds() image.Rectangle {
	return image.Rect(-g.r, -g.r, g.r + 1, g.r + 1)
}

type NoteHead struct {
	CenteredGlyph
	hollowness float64
}

func (n *NoteHead) At(x, y int) color.Color {
	α := 35.0
	xx, yy, rr := float64(x)+0.5, float64(y)+0.5, float64(n.r)
	rx := xx * math.Cos(α) - yy * math.Sin(α)
	ry := xx * math.Sin(α) + yy * math.Cos(α)
	rr2 := rr*rr
	dist2 := rx*rx + 1.25*1.25*ry*ry
	if dist2 < rr2 && dist2 >= n.hollowness * rr2 {
		return color.Opaque
	}
	return color.Transparent
}

type NoteTail struct {
	glyphBase
	sz int
	downBeam bool
}

func (t *NoteTail) Bounds() image.Rectangle {
	if t.downBeam {
		return image.Rect(0, -t.sz, t.sz + 1, 0 + 1)
	}
	return image.Rect(0, 0, t.sz + 1, t.sz + 1)
}

func (t *NoteTail) At(x, y int) color.Color {
	if x > 0 && ((t.downBeam && x + y == 0) || (!t.downBeam && x - y == 0)) {
		return color.Opaque
	}
	return color.Transparent
}

type FlatGlyph struct {
	glyphBase
	r int
}

func (f *FlatGlyph) At(x, y int) color.Color {
	if x == -2 ||
	    (y <= 2 && y >= 0 && y + x == 1) ||
	    (y < 0 && y >= -2 && y - x == -1) {
		return color.Opaque
	}
	return color.Transparent
}

func (f *FlatGlyph) Bounds() image.Rectangle {
	return image.Rect(-f.r, -f.r - 3, f.r + 1, f.r - 3 + 1)
}

type SharpGlyph struct {
	CenteredGlyph
}

func (s *SharpGlyph) At(x, y int) color.Color {
	line := y + ceil(x, 2)
	if (x == -2 || x == 2) ||
	    (line == 2 || line == -2) {
		return color.Opaque
	}
	return color.Transparent
}

type NaturalGlyph struct {
	CenteredGlyph
}

func (n *NaturalGlyph) At(x, y int) color.Color {
	line := y + divØ(x, 2)
	if (x == -2 && y < 3) ||
	    (x == 2 && y > -3) ||
	    (x > -3 && x < 3 && (line == 1 || line == -1)) {
		return color.Opaque
	}
	return color.Transparent
}

type DefaultGlyph struct {
	CenteredGlyph
}

func (d *DefaultGlyph) At(x, y int) color.Color {
	inX := (x > -3 && x < 3)
	inY := (y > -3 && y < 3)
	if (x == -3 && inY) ||
	    (x == 3 && inY) ||
	    (y == -3 && inX) ||
	    (y == 3 && inX) {
		return color.Opaque
	}
	return color.Transparent
}

type DotGlyph struct {
	CenteredGlyph
}

func (f *DotGlyph) At(x, y int) color.Color {
	dx, dy := x, y
	if dx > 0 && dx <= 2 {
		dx--
	}
	if (dy + dx >= -1 && dx + dy <= 1) &&
	    (dy - dx >= -1 && dy - dx <= 1) {
		return color.Opaque
	}
	return color.Transparent
}

func MkIcon(data []string) *image.Alpha {
	w, h := len(data[0]), len(data)
	img := image.NewAlpha(box(w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			switch data[y][x] {
			case '#':
				img.SetAlpha(x, y, color.Alpha{0xff})
			}
		}
	}
	return img
}

var IconVol *image.Alpha = MkIcon([]string{
"________________",
"___####_________",
"___#__##________",
"___#__#_#_______",
"___#__#__#______",
"___#__#___#_____",
"___#__#____##___",
"___#__#_____#___",
"___#__#_____#___",
"___#__#____##___",
"___#__#___#_____",
"___#__#__#______",
"___#__#_#_______",
"___#__##________",
"___####_________",
"________________",
})

var IconMidi *image.Alpha = MkIcon([]string{
"________________",
"################",
"__#__###_###__#_",
"__#__###_###__#_",
"__#__###_###__#_",
"__#__###_###__#_",
"__#__###_###__#_",
"__#__###_###__#_",
"__#__###_###__#_",
"__#__###_###__#_",
"__#___#___#___#_",
"__#___#___#___#_",
"__#___#___#___#_",
"__#___#___#___#_",
"__#___#___#___#_",
"__#___#___#___#_",
})

var IconWave *image.Alpha = MkIcon([]string{
"____#___________",
"____##__________",
"____##_____#____",
"___###__#__##___",
"___####_#_###___",
"__#####_######__",
"_#############_#",
"################",
"################",
"_#############_#",
"__#####_######__",
"___####_#_###___",
"___###__#__##___",
"____##_____#____",
"____##__________",
"____#___________",
})
