package main

import (
	"image"
	"image/color"
	"image/draw"
)

type MenuItem interface {
	Bounds() image.Rectangle
	Draw(dst draw.Image, r image.Rectangle)
}

type MenuWidget struct {
	// general settings
	options []MenuItem
	lastSelected int
	maxWidth int
	height int // per option

	// details of current instance
	origin int
	reply chan MenuItem
	rect image.Rectangle
	hover image.Point
	refresh chan image.Rectangle
}

type MenuString string

func (str MenuString) Bounds() image.Rectangle {
	font := G.font.luxi
	return image.Rect(0, 0, font.PixelWidth(string(str)), font.PixelHeight())
}

func (str MenuString) Draw(dst draw.Image, r image.Rectangle) {
	centre := image.Pt((r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2)
	font := G.font.luxi
	font.DrawC(dst, color.RGBA{0, 0, 0, 255}, r, string(str), centre)
}

func mkStringMenu(defaultIndex int, strings... string) MenuWidget {
	options := make([]MenuItem, len(strings))
	for i, str := range strings {
		options[i] = MenuString(str)
	}
	return mkMenu(defaultIndex, options...)
}

func mkMenu(defaultIndex int, options... MenuItem) MenuWidget {
	menu := MenuWidget{lastSelected: defaultIndex, options: options}
	for _, item := range options {
		w, h := item.Bounds().Dx(), item.Bounds().Dy()
		if w > menu.maxWidth {
			menu.maxWidth = w
		}
		if h > menu.height {
			menu.height = h
		}
	}
	return menu
}

func (menu *MenuWidget) Rect() image.Rectangle {
	return menu.rect
}

func (menu *MenuWidget) Popup(bounds image.Rectangle, refresh chan image.Rectangle, mouse image.Point) chan MenuItem {
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
	menu.rect = r.Add(image.Pt(dx, dy))

	menu.hover = mouse
	menu.reply = make(chan MenuItem)
	menu.refresh <- menu.rect.Inset(-1)
	return menu.reply
}

func (menu *MenuWidget) MouseMoved(mouse image.Point) {
	menu.hover = mouse
	menu.refresh <- image.Rect(0, 0, 0, 0)
}

func (menu *MenuWidget) RightButtonUp(mouse image.Point) {
	defer func() {
		menu.rect = image.Rect(0, 0, 0, 0)
		menu.refresh <- image.Rect(0, 0, 0, 0)
		close(menu.reply)
	}()
	if !mouse.In(menu.rect) {
		menu.reply <- nil
		return
	}
	i := mod(menu.origin + (mouse.Y - menu.rect.Min.Y) / menu.height, len(menu.options))
	menu.reply <- menu.options[i]
	menu.lastSelected = i
}

func (menu *MenuWidget) Draw(dst draw.Image, r image.Rectangle) {
	border := color.RGBA{0x88, 0x88, 0x88, 255}
	bg_norm := color.RGBA{0xee, 0xee, 0xcc, 255}
	bg_sel := color.RGBA{0xdd, 0xdd, 0xdd, 255}
	drawBorders(dst, menu.rect.Inset(-1), border, bg_norm)
	hover_i := mod(menu.origin + (menu.hover.Y - menu.rect.Min.Y) / menu.height, len(menu.options))
	ih := menu.height
	for j := 0; j < len(menu.options); j++ {
		item_r := image.Rect(menu.rect.Min.X, menu.rect.Min.Y + j*ih, menu.rect.Max.X, menu.rect.Min.Y + (j+1)*ih)
		i := mod(menu.origin + j, len(menu.options))
		if i == hover_i {
			draw.Draw(dst, item_r, &image.Uniform{bg_sel}, image.ZP, draw.Over)
		}
		menu.options[i].Draw(dst, item_r)
	}
}
