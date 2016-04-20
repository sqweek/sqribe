package main

import (
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"image/draw"
	"io/ioutil"
)

// TODO get actual DPI
const DPI = 96.0

type Font struct {
	fc *freetype.Context
	font *truetype.Font
	fontscale fixed.Int26_6
}

func NewFont(filename string, size int) (*Font, error) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	f, err := freetype.ParseFont(bytes)
	if err != nil {
		return nil, err
	}

	// should the freetype context be shared between fonts?
	font := Font{fc: freetype.NewContext(), font: f}
	font.fc.SetDPI(DPI)
	font.fc.SetFont(font.font)
	font.SetSize(size)
	return &font, nil
}

func (font *Font) SetSize(size int) {
	fsz := float64(size)
	font.fc.SetFontSize(fsz)
	font.fontscale = fixed.Int26_6(fsz * DPI * (64.0 / 72.0)) // copied from freetype.recalc()
}

/* freetype.Font functions return 26.6 fixed width ints */
func roundFix(fix fixed.Int26_6) int {
	mask := fixed.Int26_6(0x3f)
	floor := int(fix >> 6)
	frac := fix & mask
	if frac >= mask / 2 {
		floor++
	}
	return floor
}

func (font *Font) PixelWidth(str string) int {
	var previ truetype.Index
	width := fixed.Int26_6(0)
	for istr, r := range str {
		i := font.font.Index(r)
		hm := font.font.HMetric(font.fontscale, i)
		width += hm.AdvanceWidth
		if istr != 0 {
			width += font.font.Kern(font.fontscale, previ, i)
		} else {
			width += hm.LeftSideBearing
		}
		previ = i
	}
	return roundFix(width)
}

func (font *Font) PixelHeight() int {
	b := font.font.Bounds(font.fontscale)
	return 1 + roundFix(b.Max.Y - b.Min.Y)
}

func (font *Font) Draw(dst draw.Image, colour color.Color, r image.Rectangle, str string) {
	b := font.font.Bounds(font.fontscale)
	h := font.PixelHeight()
	baseline := (r.Min.Y + r.Max.Y) / 2 + h / 2 + roundFix(b.Min.Y)
	font.fc.SetDst(dst)
	font.fc.SetSrc(&image.Uniform{colour})
	font.fc.SetClip(r)
	font.fc.DrawString(str, freetype.Pt(r.Min.X + 1, baseline))
}

/* Renders a string centered at point 'pt' */
func (font *Font) DrawC(dst draw.Image, colour color.Color, clip image.Rectangle, str string, pt image.Point) {
	w := font.PixelWidth(str)
	b := font.font.Bounds(font.fontscale)
	h := font.PixelHeight()
	left := pt.X - w / 2
	baseline := pt.Y + h / 2 + roundFix(b.Min.Y)
	font.fc.SetDst(dst)
	font.fc.SetSrc(&image.Uniform{colour})
	font.fc.SetClip(clip)
	font.fc.DrawString(str, freetype.Pt(left, baseline))
}
