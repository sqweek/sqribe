package main

import (
	"image"
)

func box(width, height int) image.Rectangle {
	return image.Rectangle{Min: image.Point{0, 0}, Max: image.Point{width, height}}
}

func topV(src, container image.Rectangle) image.Rectangle {
	delta := container.Min.Y - src.Min.Y
	return src.Add(image.Point{0, delta})
}

func centerV(src, container image.Rectangle) image.Rectangle {
	mid := (container.Min.Y + container.Max.Y) / 2
	dy := src.Dy()
	half := dy / 2
	return image.Rect(src.Min.X, mid - half, src.Max.X, mid + (dy - half))
}

func rightH(src, container image.Rectangle) image.Rectangle {
	delta := container.Max.X - src.Max.X
	return src.Add(image.Point{delta, 0})
}

func leftH(src, container image.Rectangle) image.Rectangle {
	delta := container.Min.X - src.Min.X
	return src.Sub(image.Point{delta, 0})
}

func leftRect(src image.Rectangle, width int) image.Rectangle {
	return image.Rect(src.Min.X - width, src.Min.Y, src.Min.X, src.Max.Y)
}

func rightRect(src image.Rectangle, width int) image.Rectangle {
	return image.Rect(src.Max.X, src.Min.Y, src.Max.X + width, src.Max.Y)
}

func aboveRect(src image.Rectangle, height int) image.Rectangle {
	return image.Rect(src.Min.X, src.Min.Y - height, src.Max.X, src.Min.Y)
}

func belowRect(src image.Rectangle, height int) image.Rectangle {
	return image.Rect(src.Min.X, src.Max.Y, src.Max.X, src.Max.Y + height)
}

func tickRect(r image.Rectangle, bottom bool, x, size int) image.Rectangle {
	if bottom {
		return image.Rect(x, r.Max.Y - size, x + 1, r.Max.Y)
	}
	return image.Rect(x, r.Min.Y, x + 1, r.Min.Y + size)
}

func padPt(center image.Point, w, h int) image.Rectangle {
	return image.Rect(center.X - w, center.Y - h, center.X + w + 1, center.Y + h + 1)
}

func padRect(r image.Rectangle, w, h int) image.Rectangle {
	return image.Rect(r.Min.X - w, r.Min.Y - h, r.Max.X + w, r.Max.Y + h)
}

func vrect(r image.Rectangle, x int) image.Rectangle {
	return image.Rect(x, r.Min.Y, x + 1, r.Max.Y)
}
