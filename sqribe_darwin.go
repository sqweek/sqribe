package main

import (
	"github.com/skelterjohn/go.wde"
)

func init() {
	eventFilter = ctrlClicks
}

func isCtrl(key string) bool {
	return key == wde.KeyLeftControl || key == wde.KeyRightControl
}


// converts left-click events to right-clicks while control is depressed
func ctrlClicks(in <-chan interface{}) <-chan interface{} {
	out := make(chan interface{})
	go func() {
		ctrl := false
		conv := false
		for ei := range in {
			if e, ok := ei.(wde.KeyDownEvent); ok && isCtrl(e.Key) {
				ctrl = true
			} else if e, ok := ei.(wde.KeyUpEvent); ok && isCtrl(e.Key) {
				ctrl = false
			}
			switch e := ei.(type) {
			case wde.MouseDownEvent:
				if ctrl && e.Which == wde.LeftButton {
					conv = true
					e.Which = wde.RightButton
					out <- e
				} else {
					out <- ei
				}
			case wde.MouseUpEvent:
				if conv && e.Which == wde.LeftButton {
					e.Which = wde.RightButton
					conv = false
					out <- e
				} else {
					out <- ei
				}
			case wde.MouseDraggedEvent:
				if conv && e.Which == wde.LeftButton {
					e.Which = wde.RightButton
					out <- e
				} else {
					out <- ei
				}
			default:
				out <- ei
			}
		}
		close(out)
	}()
	return out
}