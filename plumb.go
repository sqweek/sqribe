package main

type subReq struct {
	key interface{}
	c chan interface{}
}

type PlumbPort struct {
	C chan<- interface{}
	c chan interface{}
	sub chan subReq
	unsub chan interface{}
}

func MkPort() *PlumbPort {
	plumb := PlumbPort{c: make(chan interface{})}
	plumb.C = plumb.c
	plumb.sub = make(chan subReq)
	plumb.unsub = make(chan interface{})
	go plumb.broadcast()
	return &plumb
}

func (plumb *PlumbPort) Sub(origin interface{}, subchan chan interface{}) {
	plumb.sub <- subReq{origin, subchan}
}

func (plumb *PlumbPort) Unsub(origin interface{}) {
	plumb.unsub <- origin
}

func (plumb *PlumbPort) broadcast() {
	subs := make(map[interface{}]chan interface{})
	for {
		select {
		case sub := <-plumb.sub:
			c, ok := subs[sub.key]
			if ok {
				close(c)
			}
			subs[sub.key] = sub.c
		case key := <-plumb.unsub:
			c, ok := subs[key]
			if ok {
				close(c)
				delete(subs, key)
			}
		case ev, ok := <-plumb.c:
			if !ok {
				for _, c := range subs {
					close(c)
				}
				return
			}
			for _, c := range subs {
				c <- ev
			}
		}
	}
}
