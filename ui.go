package main

import (
	"image/draw"
	"image/color"
	"image"
	"time"
	"fmt"
)

type Drawable interface {
	Rect() image.Rectangle
	Draw(draw.Image, image.Rectangle)
}


type BpmWidget struct {
	bpm float64
	hits []time.Time
	area image.Rectangle
}

func (bw *BpmWidget) Rect() image.Rectangle {
	return bw.area
}

func (bw *BpmWidget) Draw(dst draw.Image, r image.Rectangle) {
	bw.area = r
	bg := color.RGBA{0xcc, 0xcc, 0xcc, 0xff}
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Src)
	RenderString(dst, color.Black, r, fmt.Sprintf("%f", bw.bpm))
}

func (bw *BpmWidget) Hit() float64 {
	cutoff := time.Second*3
	now := time.Now()
	if len(bw.hits) > 0 && now.After(bw.hits[len(bw.hits)-1].Add(cutoff)) {
		bw.hits = bw.hits[0:0]
	}
	bw.hits = append(bw.hits, now)
	if len(bw.hits) > 1 {
		bw.bpm = 60.0 * float64(time.Second) / (float64(bw.hits[len(bw.hits)-1].Sub(bw.hits[0])) / float64(len(bw.hits) - 1))
		return bw.bpm
	}
	return 0.0
}
