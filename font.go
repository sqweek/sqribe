package main

import (
	"code.google.com/p/freetype-go/freetype"
	"image/color"
	"image/draw"
	"image"
	"io/ioutil"
)

var fc *freetype.Context

func FontInit() error {
	filename := "/usr/lib/go/site/src/code.google.com/p/freetype-go/luxi-fonts/luxisr.ttf"
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	font, err := freetype.ParseFont(bytes)
	if err != nil {
		return err
	}

	fc = freetype.NewContext()
	// TODO get actual DPI
	fc.SetDPI(96)
	fc.SetFont(font)
	fc.SetFontSize(10)
	return nil
}

func RenderString(dst draw.Image, colour color.Color, r image.Rectangle, str string) {
	fc.SetDst(dst)
	fc.SetSrc(&image.Uniform{colour})
	fc.SetClip(r)
	fc.DrawString(str, freetype.Pt(r.Min.X + 10, r.Min.Y + 10))
}