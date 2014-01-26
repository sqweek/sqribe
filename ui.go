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

type BpmTracker struct {
	t0 time.Time
	Hits []time.Duration
}

func (bt *BpmTracker) Clear() {
	bt.Hits = bt.Hits[0:0]
}

func (bt *BpmTracker) AppendTime(t time.Time) {
	if len(bt.Hits) == 0 {
		bt.t0 = t
	}
	bt.Hits = append(bt.Hits, t.Sub(bt.t0))
}

func (bt *BpmTracker) Append(d time.Duration) {
	bt.Hits = append(bt.Hits, d)
}

func (bt *BpmTracker) LastTime() time.Time {
	if len(bt.Hits) == 0 {
		return time.Time{}
	}
	return bt.t0.Add(bt.Hits[len(bt.Hits) - 1])
}

func (bt *BpmTracker) Bpm() float64 {
	if len(bt.Hits) <= 1 {
		return 0.0
	}
	n := len(bt.Hits) - 1
	return 60.0 * float64(time.Second) / (float64(bt.Hits[n] - bt.Hits[0]) / float64(n))
}

type BpmWidget struct {
	BpmTracker
	bpm float64
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
	if now.After(bw.LastTime().Add(cutoff)) {
		bw.Clear()
	}
	bw.AppendTime(now)
	bpm := bw.Bpm()
	if bpm != 0.0 {
		bw.bpm = bpm
	}
	return bpm
}

func (bw *BpmWidget) SetBpm(bpm float64) {
	bw.bpm = bpm
}
