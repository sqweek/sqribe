package main

import (
	"fmt"
	"image/draw"
	"image/color"
	"image"
	"time"

	"github.com/sqweek/go.wde"

	. "sqweek.net/sqribe/core/data"
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

type SliderWidget struct {
	WidgetCore
	data *BoundFloat
	vertical bool
}

type SliderMoved struct {
	Slider *SliderWidget
}

func NewSlider(data *BoundFloat, vert bool, refresh chan Widget) *SliderWidget {
	changes := make(chan interface{})
	slider := SliderWidget{WidgetCore{refresh: refresh}, data, vert}
	data.Port().Sub(&slider, changes)
	go func() {
		for _ = range changes {
			refresh <- &slider
		}
	}()
	return &slider
}

func (s *SliderWidget) LeftClick(mouse image.Point) {
	s.data.Shunt(-0.05)
}

func (s *SliderWidget) RightClick(mouse image.Point) {
	s.data.Shunt(0.05)
}

func (s *SliderWidget) Draw(dst draw.Image, r image.Rectangle) {
	bg := color.RGBA{0xcc, 0xcc, 0xcc, 0xff}
	fg := color.RGBA{0x00, 0x00, 0x00, 255}
	s.r = r
	posn := s.data.Posn()
	if s.vertical {
		drawVertSlider(dst, r, bg, fg, posn)
	} else {
		drawHorzSlider(dst, r, bg, fg, posn)
	}
}

func drawHorzSlider(dst draw.Image, r image.Rectangle, bg, fg color.Color, posn float64) {
	mid := r.Min.Y + r.Dy() / 2
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Over)
	draw.Draw(dst, image.Rect(r.Min.X, mid, r.Max.X, mid + 1), &image.Uniform{fg}, image.ZP, draw.Over)
	x := int(float64(r.Min.X) + posn * float64(r.Dx()) + 0.5)
	draw.Draw(dst, image.Rect(x - 1, r.Min.Y + 1, x + 2, r.Max.Y - 2), &image.Uniform{fg}, image.ZP, draw.Over)
}

func drawVertSlider(dst draw.Image, r image.Rectangle, bg, fg color.Color, posn float64) {
	mid := r.Min.X + r.Dx() / 2
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Over)
	draw.Draw(dst, image.Rect(mid, r.Min.Y, mid + 1, r.Max.Y), &image.Uniform{fg}, image.ZP, draw.Over)
	y := int(float64(r.Max.Y) - posn * float64(r.Dy()) + 0.5)
	x := r.Dx() / 2 - 1
	draw.Draw(dst, image.Rect(mid - x, y - 1, mid + x + 1, y + 2), &image.Uniform{fg}, image.ZP, draw.Over)
}
