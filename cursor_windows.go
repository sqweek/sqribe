package main

import (
	"fmt"
	"github.com/AllenDang/w32"
	"github.com/sqweek/go.wde"
	"github.com/sqweek/go.wde/win"
)

type WinCursorCtl struct {
	window *win.Window
	current Cursor
}

var wincursors map[Cursor] w32.HCURSOR

func stdCursor(idc int) w32.HCURSOR {
	res := w32.LoadCursor(0, w32.MakeIntResource(uint16(idc)))
	fmt.Println("stdCursor", idc, res)
	return res
}

func init() {
	wincursors = make(map[Cursor]w32.HCURSOR)
	wincursors[NormalCursor] = stdCursor(w32.IDC_ARROW)
	wincursors[ResizeHCursor] = stdCursor(w32.IDC_SIZEWE)
	wincursors[ResizeLCursor] = stdCursor(w32.IDC_SIZEWE)
	wincursors[ResizeRCursor] = stdCursor(w32.IDC_SIZEWE)
	wincursors[GrabCursor] = stdCursor(w32.IDC_HAND)
}

func (cc *WinCursorCtl) Set(c Cursor) {
	if c == cc.current {
		return
	}
	cc.current = c
	f := func() {
		fmt.Println("w32.SetCursor", c)
		w32.SetCursor(wincursors[c])
	}
	fmt.Println("CursorCtl.Set", c, &f)
	// note OnUiThread is local go.wde hack
	cc.window.OnUiThread(f)
}

func NewCursorCtl(wdewin wde.Window) CursorCtl {
	window, ok := wdewin.(*win.Window)
	if !ok {
		panic("window not win.Window")
	}
	return &WinCursorCtl{window, NormalCursor}
}
