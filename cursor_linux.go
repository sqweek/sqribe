package main

import (
	"github.com/BurntSushi/xgb/xproto"
	"github.com/BurntSushi/xgbutil"
	"github.com/BurntSushi/xgbutil/xcursor"
	"github.com/BurntSushi/xgbutil/xwindow"

	"github.com/sqweek/go.wde"
	wdexgb "github.com/sqweek/go.wde/xgb"

	"fmt"
)

var xcursors map[Cursor] xproto.Cursor

func mkCursor(X *xgbutil.XUtil, xcur uint16) xproto.Cursor {
	cur, err := xcursor.CreateCursor(X, xcur)
	if err != nil {
		fmt.Printf("warning: X CreateCursor failed for cursor %d\n", xcur)
	}
	return cur
}

// /usr/lib/go/site/src/github.com/BurntSushi/xgbutil/xcursor/cursordef.go
func initCursors(X *xgbutil.XUtil) {
	xcursors = make(map[Cursor] xproto.Cursor)
	xcursors[NormalCursor] = xcursor.XCursor
	xcursors[ResizeHCursor] = mkCursor(X, xcursor.SBHDoubleArrow)
	xcursors[ResizeLCursor] = mkCursor(X, xcursor.LeftSide)
	xcursors[ResizeRCursor] = mkCursor(X, xcursor.RightSide)
	xcursors[GrabCursor] = mkCursor(X, xcursor.Hand2)
}

type XgbCursorCtl struct {
	xwin *xwindow.Window
	current Cursor
}

func (cc *XgbCursorCtl) Set(c Cursor) {
	if c == cc.current {
		return
	}
	cc.xwin.Change(xproto.CwCursor, uint32(xcursors[c]))
	cc.current = c
}

func NewCursorCtl(wdewin wde.Window) CursorCtl {
	xgbwin, ok := wdewin.(*wdexgb.Window)
	if !ok {
		panic("wdewin is not *wdexgb.Window")
	}
	if len(xcursors) == 0 {
		// note Win() is a local wde hack to get the native window handle
		initCursors(xgbwin.Win().X)
	}
	return &XgbCursorCtl{xwin: xgbwin.Win()}
}
