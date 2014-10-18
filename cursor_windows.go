package main

import (
	"github.com/skelterjohn/go.wde"
)

type WinCursorCtl struct {
}

func (cc *WinCursorCtl) Set(c Cursor) {
	// TODO
}

func NewCursorCtl(wdewin wde.Window) CursorCtl {
	return &WinCursorCtl{}
}
