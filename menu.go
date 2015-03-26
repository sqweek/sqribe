package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
)

type MenuOps interface {
	Bounds(item interface{}) image.Rectangle
	Draw(item interface{}, dst draw.Image, r image.Rectangle)
}

type MenuWidget struct {
	WidgetCore

	// general settings
	ops MenuOps
	options []interface{}
	lastSelected int
	maxWidth int
	height int // per option

	// details of current instance
	origin int
	reply chan interface{}
	hover image.Point
}

type StringMenuOps struct {
	toStr func(interface{}) string
}

func (ops StringMenuOps) str(item interface{}) string {
	if ops.toStr == nil {
		str, ok := item.(string)
		if ok {
			return str
		}
		return item.(fmt.Stringer).String()
	}
	return ops.toStr(item)
}

func (ops StringMenuOps) Bounds(item interface{}) image.Rectangle {
	font := G.font.luxi
	return image.Rect(0, 0, font.PixelWidth(ops.str(item)), font.PixelHeight())
}

func (ops StringMenuOps) Draw(item interface{}, dst draw.Image, r image.Rectangle) {
	centre := image.Pt((r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2)
	font := G.font.luxi
	font.DrawC(dst, color.RGBA{0, 0, 0, 255}, r, ops.str(item), centre)
}

func mkMenu(ops MenuOps, defaultIndex int, options... interface{}) MenuWidget {
	menu := MenuWidget{lastSelected: defaultIndex, ops: ops, options: options}
	for _, item := range options {
		r := ops.Bounds(item)
		if r.Dx() > menu.maxWidth {
			menu.maxWidth = r.Dx()
		}
		if r.Dy() > menu.height {
			menu.height = r.Dy()
		}
	}
	return menu
}

func (menu *MenuWidget) Popup(bounds image.Rectangle, refresh chan Widget, mouse image.Point) chan interface{} {
	menu.refresh = refresh
	w := menu.maxWidth
	ih := menu.height
	h := ih * len(menu.options)
	relTarget := image.Point{w / 2, ih * menu.lastSelected + ih / 2}
	target := mouse.Sub(relTarget)
	r := image.Rectangle{target, target.Add(image.Pt(w, h))}
	min := r.Min.Sub(bounds.Min)
	max := bounds.Max.Sub(r.Max)
	dx := 0
	if min.X < 0 {
		dx = -min.X
	} else if max.X < 0 {
		dx = max.X
	}
	menu.origin = 0
	if max.Y < 0 {
		menu.origin = -ceil(-max.Y, ih)
	} else if min.Y < 0 {
		menu.origin = ceil(-min.Y, ih)
	}
	dy := menu.origin * ih
	menu.r = r.Add(image.Pt(dx, dy))

	menu.hover = mouse
	menu.reply = make(chan interface{})
	menu.refresh <- menu
	return menu.reply
}

func (menu *MenuWidget) Drag(mouse image.Point, finished bool, moved bool) bool {
	contained := mouse.In(menu.r)
	if !finished {
		menu.hover = mouse
		menu.refresh <- menu
		return contained
	}

	defer func() {
		menu.r = image.Rect(0, 0, 0, 0)
		menu.refresh <- menu
		close(menu.reply)
	}()
	if !contained {
		menu.reply <- nil
		return false
	}
	i := mod(menu.origin + (mouse.Y - menu.r.Min.Y) / menu.height, len(menu.options))
	menu.reply <- menu.options[i]
	menu.lastSelected = i
	return contained
}

func (menu *MenuWidget) Draw(dst draw.Image, r image.Rectangle) {
	border := color.RGBA{0x88, 0x88, 0x88, 255}
	bg_norm := color.RGBA{0xee, 0xee, 0xcc, 255}
	bg_sel := color.RGBA{0xdd, 0xdd, 0xdd, 255}
	drawBorders(dst, menu.r.Inset(-1), border, bg_norm)
	hover_i := mod(menu.origin + (menu.hover.Y - menu.r.Min.Y) / menu.height, len(menu.options))
	ih := menu.height
	for j := 0; j < len(menu.options); j++ {
		item_r := image.Rect(menu.r.Min.X, menu.r.Min.Y + j*ih, menu.r.Max.X, menu.r.Min.Y + (j+1)*ih)
		i := mod(menu.origin + j, len(menu.options))
		if i == hover_i {
			draw.Draw(dst, item_r, &image.Uniform{bg_sel}, image.ZP, draw.Over)
		}
		menu.ops.Draw(menu.options[i], dst, item_r)
	}
}
