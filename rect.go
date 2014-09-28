package main

import (
	"image"
)

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
