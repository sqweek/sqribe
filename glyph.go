package main

import (
	"image/color"
	"image/draw"
	"image"
	"math"

	"github.com/sqweek/go-image.svg"
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

	p.TrebleClef = trebleSvg.ScaleToHeight(71 * sz / 10)
	p.BassClef = bassSvg.ScaleToHeight(33 * sz / 10)
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

type SvgPath struct {
	x, y float64 // hotspot as percentage of width/height [0,1]
	path []svg.Segment
	bounds svg.Bounds
}

// height is the non-border height
func (p SvgPath) ScaleToHeight(height int) Glyph {
	width := int(math.Ceil(p.bounds.WidthForHeight(float64(height))))
	min := image.Pt(-int(float64(width) * p.x + 0.5), -int(float64(height) * p.y + 0.5))
	r := image.Rectangle{min, min.Add(image.Pt(width, height))}
	return svg.PathMask(p.path, p.bounds, r)
}

func mustMkSvgPath(x, y float64, path string) (p SvgPath) {
	var err error
	p.x, p.y = x, y
	p.path, err = svg.ParsePath(path)
	p.bounds = svg.PathBounds(p.path)
	if err != nil {
		panic(err)
	}
	return
}

var trebleSvg = mustMkSvgPath(0.53, 0.46,
	"m278.89 840.5c0 23.811 27.144 51.133 48.193 55.108 0 0 22.508 5.6134 41.03 1.1224 3.3461 0.75146 59.498-9.5416 60.62-84.755l-11.843-91.536c7.8919-1.3481 33.643-13.763 41.592-18.478 21.812-12.943 31.355-31.636 39.852-45.465 9.5416-20.768 15.155-34.946 15.155-57.556 0-57.185-46.341-103.54-103.5-103.54-8.2386 0-16.243 0.97722-23.934 2.7962l-10.216-73.464c38.684-43.006 77.897-95.869 91.065-144.31 12.91-63.989 6.7357-107.21-27.504-125.73-31.432-7.2969-69.589 35.643-92.052 97.666-14.594 33.116 3.9976 96.385 12.349 122.36-61.528 58.983-122.74 101.28-129.1 203.51 0 80.053 64.874 144.94 144.89 144.94 7.6452 0 20.275 0.179 34.643-1.6835l6.7583 91.412s2.593 66.862-48.293 75.922c-20.196 3.5928-39.674 7.7565-34.07-5.6118 15.739-8.1967 34.07-21.263 34.07-42.704 0-26.683-20.085-48.316-44.849-48.316-24.774 0-44.859 21.633-44.859 48.316zm90.975-409.14 9.5303 68.634c-42.22 13.057-72.901 52.426-72.901 98.946 0 52.649 62.945 77.202 61.239 64.706-24.54-10.587-33.633-22.486-33.633-50.911 0-32.129 21.958-59.115 51.673-66.806l22.665 163.41c-5.0974 1.3804-10.598 2.5479-16.558 3.4799-86.249-1.7287-126.79-47.632-126.79-127.68 0.002-44.846 52.809-96.576 104.77-153.77zm45.197 275.97-22.599-162.78c3.4574-0.5386 7.0163-0.83047 10.632-0.83047 38.1 0 68.994 30.905 68.994 69.017-0.76275 30.67-2.2237 76.336-57.027 94.59zm-48.06-342.71c-15.548-52.459-1.4932-78.603 7.6323-98.699 21.565-47.452 40.537-64.706 59.857-80.108 25.124 9.8561 13.784 80.108 10.012 80.108-11.831 41.446-54.276 74.395-77.502 98.699z")

var bassSvg = mustMkSvgPath(0.5, 0.58,
	"m190.85 451.25c11.661 14.719 32.323 24.491 55.844 24.491 36.401 0 65.889-23.372 65.889-52.214s-29.488-52.214-65.889-52.214c-20.314 4.1522-28.593 9.0007-33.143-2.9091 17.976-54.327 46.918-66.709 96.546-66.709 65.914 0 96.969 59.897 96.969 142.97-18.225 190.63-205.95 286.75-246.57 316.19 5.6938 13.103 5.3954 12.631 5.3954 12.009 189.78-86.203 330.69-204.43 330.69-320.74 0-92.419-58.579-175.59-187.72-172.8-77.575 0-170.32 86.203-118 171.93zm328.1-89.88c0 17.852 14.471 32.323 32.323 32.323s32.323-14.471 32.323-32.323-14.471-32.323-32.323-32.323-32.323 14.471-32.323 32.323zm0 136.75c0 17.852 14.471 32.323 32.323 32.323s32.323-14.471 32.323-32.323-14.471-32.323-32.323-32.323-32.323 14.471-32.323 32.323z")

var IconTreble Glyph = trebleSvg.ScaleToHeight(16)
var IconBass Glyph = bassSvg.ScaleToHeight(16)

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
