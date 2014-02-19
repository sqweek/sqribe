package main

type Cursor int

const (
	NormalCursor Cursor = iota + 1
	ResizeHCursor
	ResizeLCursor
	ResizeRCursor
)

type CursorCtl interface {
	Set(c Cursor)
}
