package main

import (
	"fmt"
	"image/draw"
	"image/color"
	"image"
	"time"
)

type DragFn func(image.Point, bool) bool

type Widget interface {
	Rect() image.Rectangle
}

type Hoverable interface {
	MouseMoved(image.Point)
}

type LeftClickable interface {
	LeftClick(image.Point)
}

type RightClickable interface {
	RightClick(image.Point)
}

type MouseDraggable interface {
	MouseDragged(image.Point)
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
	event chan<- interface{}
	refresh chan Widget
}

func (w *WidgetCore) Rect() image.Rectangle {
	return w.r
}

func (w *WidgetCore) publish(ev interface{}) {
	if w.refresh != nil {
		w.refresh <- w
	}
	if w.event != nil {
		w.event <- ev
	}
}

type SliderWidget struct {
	WidgetCore
	α float64
}

type SliderMoved struct {
	Slider *SliderWidget
}

func NewSlider(α float64, refresh chan Widget) *SliderWidget {
	return &SliderWidget{WidgetCore{refresh: refresh}, α}
}

// α is on range [0,1]
func (s *SliderWidget) SetSlider(α float64) {
	δ := α - s.α
	if δ != 0 {
		s.α = α
		s.publish(SliderMoved{s})
	}
}

func (s *SliderWidget) Value() float64 {
	return s.α
}

func (s *SliderWidget) Shunt(delta float64) {
	α := s.α + delta
	if α < 0 {
		α = 0
	}
	if α > 1 {
		α = 1
	}
	s.SetSlider(α)
}

func (s *SliderWidget) LeftClick(mouse image.Point) {
	s.Shunt(-0.05)
}

func (s *SliderWidget) RightClick(mouse image.Point) {
	s.Shunt(0.05)
}

func (s *SliderWidget) Draw(dst draw.Image, r image.Rectangle) {
	bg := color.RGBA{0xcc, 0xcc, 0xcc, 0xff}
	fg := color.RGBA{0x00, 0x00, 0x00, 255}
	s.r = r
	mid := r.Min.Y + r.Dy() / 2
	draw.Draw(dst, r, &image.Uniform{bg}, image.ZP, draw.Over)
	draw.Draw(dst, image.Rect(r.Min.X, mid, r.Max.X, mid + 1), &image.Uniform{fg}, image.ZP, draw.Over)
	x := int(float64(r.Min.X) + s.α * float64(r.Dx()) + 0.5)
	draw.Draw(dst, image.Rect(x - 1, r.Min.Y + 1, x + 2, r.Max.Y - 2), &image.Uniform{fg}, image.ZP, draw.Over)
}
