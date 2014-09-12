package main

type PlumbPort struct {
	C chan<- interface{}
	c chan interface{}
	Sub chan chan interface{}
	Unsub chan chan interface{}
}

func MkPort() *PlumbPort {
	plumb := PlumbPort{c: make(chan interface{})}
	plumb.C = plumb.c
	plumb.Sub = make(chan chan interface{})
	plumb.Unsub = make(chan chan interface{})
	go plumb.broadcast()
	return &plumb
}

func (plumb *PlumbPort) broadcast() {
	subs := make(map[chan interface{}]bool)
	for {
		select {
		case c := <-plumb.Sub:
			subs[c] = true
		case c := <-plumb.Unsub:
			close(c)
			delete(subs, c)
		case ev, ok := <-plumb.c:
			if !ok {
				for c, _ := range subs {
					close(c)
				}
				return
			}
			for c, _ := range subs {
				c <- ev
			}
		}
	}
}
