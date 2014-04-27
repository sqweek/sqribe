package main

type Cursor int

const (
	NormalCursor Cursor = iota + 1
	ResizeHCursor
	ResizeLCursor
	ResizeRCursor
	GrabCursor
)

type CursorCtl interface {
	Set(c Cursor)
}
