package main

import (
	"fmt"
	"image/draw"
	"image/color"
	"image"
	"sort"
	"time"

	"github.com/skelterjohn/go.wde"
)

type DragFn func(pos image.Point, finished, moved bool) bool

type Widget interface {
	Rect() image.Rectangle
}

type Hoverable interface {
	MouseMoved(image.Point) wde.Cursor
}

type LeftDraggable interface {
	LeftButtonDown(image.Point) DragFn
}

type RightDraggable interface {
	RightButtonDown(image.Point) DragFn
}

type LeftClickable interface {
	LeftClick(image.Point)
}

type RightClickable interface {
	RightClick(image.Point)
}

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
	G.font.luxi.Draw(dst, color.Black, r, fmt.Sprintf("%f", bw.bpm))
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

type WidgetCore struct {
	r image.Rectangle
	refresh chan Widget
}

func (w *WidgetCore) Rect() image.Rectangle {
	return w.r
}

func (w *WidgetCore) publish(ev interface{}) {
	if w.refresh != nil {
		w.refresh <- w
	}
}

func drawHorzSlider(dst draw.Image, r image.Rectangle, fg color.Color, posn float64) {
	mid := r.Min.Y + r.Dy() / 2
	draw.Draw(dst, image.Rect(r.Min.X, mid, r.Max.X, mid + 1), &image.Uniform{fg}, image.ZP, draw.Over)
	x := int(float64(r.Min.X) + posn * float64(r.Dx()) + 0.5)
	draw.Draw(dst, image.Rect(x - 1, r.Min.Y + 1, x + 2, r.Max.Y - 2), &image.Uniform{fg}, image.ZP, draw.Over)
}

func drawVertSlider(dst draw.Image, r image.Rectangle, fg color.Color, posn float64) {
	mid := r.Min.X + r.Dx() / 2
	draw.Draw(dst, image.Rect(mid, r.Min.Y, mid + 1, r.Max.Y), &image.Uniform{fg}, image.ZP, draw.Over)
	y := int(float64(r.Max.Y) - posn * float64(r.Dy()) + 0.5)
	x := r.Dx() / 2 - 1
	draw.Draw(dst, image.Rect(mid - x, y - 1, mid + x + 1, y + 2), &image.Uniform{fg}, image.ZP, draw.Over)
}

type ColourBar struct {
	pts []ColourPoint
}

type ColourPoint struct {
	x float64
	col color.Color
}

func (cb ColourBar) At(x float64) color.Color {
	i := sort.Search(len(cb.pts), func(i int)bool { return x <= cb.pts[i].x })
	if i >= len(cb.pts) {
		return cb.pts[len(cb.pts)-1].col
	} else if i == 0 {
		return cb.pts[i].col
	}
	α := (x - cb.pts[i-1].x) / (cb.pts[i].x - cb.pts[i-1].x)
	r0, g0, b0, a0 := cb.pts[i-1].col.RGBA()
	r1, g1, b1, a1 := cb.pts[i].col.RGBA()
	f0, f1 := a0/255, a1/255
	θ := func(a, b uint32)uint8 { return uint8((1-α)*float64(a/f0) + α*float64(b/f1)) }
	c := color.RGBA{θ(r0, r1), θ(g0, g1), θ(b0, b1), θ(a0, a1)}
	return c
}
