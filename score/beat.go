package score

import (
	"fmt"
	"math"
	"math/big"

	. "sqweek.net/sqribe/core/types"
)

type BeatRange struct {
	First, Last *BeatRef
}

func (r BeatRange) MinFrame() FrameN {
	return r.First.frame
}

func (r BeatRange) MaxFrame() FrameN {
	return r.Last.frame
}

type BeatRef struct {
	prev, next *BeatRef
	frame FrameN
}

type BeatChanged struct {
}

func (beat *BeatRef) Frame() FrameN {
	return beat.frame
}

func (beat *BeatRef) Prev() *BeatRef {
	return beat.prev
}

func (beat *BeatRef) Next() *BeatRef {
	return beat.next
}

func (beat *BeatRef) LPrev() *BeatRef {
	if beat.prev == nil {
		return beat
	}
	return beat.prev
}

func (beat *BeatRef) LNext() *BeatRef {
	if beat.next == nil {
		return beat
	}
	return beat.next
}

func (beat *BeatRef) Walk(Δbeat int) *BeatRef {
	b := beat
	if Δbeat < 0 {
		for i := 0; i > Δbeat; i-- {
			b = b.LPrev()
		}
	} else {
		for i := 0; i < Δbeat; i++ {
			b = b.LNext()
		}
	}
	return b
}

func (beat *BeatRef) BeatNum() int {
	i, b := 0, beat
	for b.prev != nil {
		i, b = i + 1, b.prev
	}
	return i
}

func sub(low, high *BeatRef) (d int) {
	for b := low; b != nil && b.frame < high.frame; b = b.next {
		d++
	}
	return d
}

/* calculates (b1 - b2); ie. the signed number of beats in between them */
func (b1 *BeatRef) Subtract(b2 *BeatRef) int {
	if b1.frame > b2.frame {
		return sub(b2, b1)
	} else if b1.frame < b2.frame {
		return -sub(b1, b2)
	}
	return 0
}

func (score *Score) Shunt(br BeatRange, Δbeat int) BeatRange {
	if Δbeat == 0 {
		return br
	}
	nbeats := br.Last.Subtract(br.First)
	if Δbeat < 0 {
		b0 := br.First.Walk(Δbeat)
		bN := b0.Walk(nbeats)
		return BeatRange{b0, bN}
	} else {
		bN := br.Last.Walk(Δbeat)
		b0 := bN.Walk(-nbeats)
		return BeatRange{b0, bN}
	}
}

func (score *Score) MvBeat(beat *BeatRef, newFrame FrameN) bool {
	orig := beat.frame
	changed := (newFrame != orig)
	if changed {
		beat.frame = newFrame
		score.plumb.C <- BeatChanged{}
	}
	return changed
}

func (score *Score) ToFrame(pt BeatPoint) (FrameN, bool) {
	b := pt.Beat()
	b2 := b.next
	α := pt.Offsetf()
	if b2 == nil {
		// last beat. any point past this is clipped to the beat's frame
		return b.frame, α < 1e-6
	}
	return FrameN((1 - α) * float64(b.frame) + α * float64(b2.frame)), true
}

/* returns a fractional beat, and true if it is within the defined beat range */
func (score *Score) ToBeat(frame FrameN) (BeatPoint, bool) {
	if frame < score.beat0.frame || frame > score.beatN.frame {
		/* should perhaps extrapolate based on bpm... */
		return BeatPt{nil, 0.0}, false
	}
	for b := score.beat0; ; b = b.next {
		if b.next == nil {
			return BeatPt{b, 0.0}, true
		}
		if b.frame <= frame && frame <= b.next.frame {
			α := float64(frame - b.frame) / float64(b.next.frame - b.frame)
			return BeatPt{b, α}, true
		}
	}
}

func (score *Score) BeatFrames() []FrameN {
	f := make([]FrameN, 0)
	for b := score.beat0; b != nil; b = b.next {
		f = append(f, b.frame)
	}
	return f
}

func (score *Score) HasBeats() bool {
	return score.beat0 != nil
}

func beatList(f []FrameN) (hd, tl *BeatRef) {
	if len(f) == 0 {
		return nil, nil
	}
	hd = &BeatRef{nil, nil ,f[0]}
	tl = hd
	for i := 1; i < len(f); i++ {
		b := &BeatRef{tl, nil, f[i]}
		tl.next = b
		tl = b
	}
	return hd, tl
}

func (score *Score) LoadBeats(f []FrameN) {
	hd, tl := beatList(f)
	score.beat0, score.beatN = hd, tl
	for b := score.beat0; b != nil; b = b.next {
		pf, nf := FrameN(-1), FrameN(-1)
		if b.prev != nil { pf = b.prev.frame }
		if b.next != nil { nf = b.next.frame }
		fmt.Println(b.frame, pf, "<- ->", nf)
	}
	for b := score.beat0; b != nil; b = b.next {
		for b2 := score.beat0; b2 != nil; b2 = b2.next {
			fmt.Println(b.frame, "-", b2.frame, "=", b.Subtract(b2))
		}
	}

	score.plumb.C <- BeatChanged{}
}

func (score *Score) AddBeat(frame FrameN) {
	if score.addBeat(frame) {
		score.plumb.C <- BeatChanged{}
	}
}

func (score *Score) addBeat(frame FrameN) bool {
	if score.beat0 == nil {
		score.beat0 = &BeatRef{nil, nil, frame}
		score.beatN = score.beat0
		return true
	}
	tolerance := FrameN(10000) //XXX should be based on sample rate/bpm
	for b := score.beat0; b != nil; b = b.next {
		if frame == b.frame {
			return false
		}
		if frame > b.frame && (b.next == nil || frame < b.next.frame) {
			if frame - b.frame <= tolerance {
				b.frame = frame
			} else if b.next == nil || (b.next.frame - frame > tolerance) {
				ref := &BeatRef{b, b.next, frame}
				ref.prev.next = ref
				if ref.next != nil {
					ref.next.prev = ref
				} else {
					score.beatN = ref
				}
			} else {
				b.next.frame = frame
			}
			return true
		}
	}
	panic("unreachable")
}

func (score *Score) NearestBeat(frame FrameN) *BeatRef {
	for b := score.beat0; b != nil; b = b.next {
		if frame == b.frame || b.next == nil {
			return b
		}
		if frame < b.next.frame {
			mid := (b.frame + b.next.frame) / 2
			if frame < mid {
				return b
			} else {
				return b.next
			}
		}
	}
	return nil
}

func (score *Score) Quantize(beat BeatPoint) (*BeatRef, *big.Rat) {
	best := big.NewRat(0, 1)
	frac := beat.Offsetf()
	minErr := frac
	for _, i := range([]int{2, 3}) { // , 5}) { //, 7}) {
		for denom := int64(i); denom <= 8; denom <<= 1 {
			for num := int64(1); num < denom; num++ {
				r := big.NewRat(num, denom)
				/* TODO account for picked beats in error measure */
				f, _ := r.Float64()
				d := math.Abs(f - frac)
				if d < minErr {
					minErr = d
					best = r
				}
			}
		}
	}
	b := beat.Beat()
	if 1 - frac < minErr {
		b = b.LNext()
		best = big.NewRat(0, 1)
	}
	return b, best
}

type QuantizeBeats struct {
	beats BeatRange
	nb int // number of divisions
	df float64
	Error *FrameN
}

func (q QuantizeBeats) Nop() bool {
	return q.beats.First == nil || q.beats.Last == nil
}

func (q QuantizeBeats) AvgFramesPerBeat() FrameN {
	return FrameN(float64(q.beats.Last.frame - q.beats.First.frame + 1) / float64(q.nb))
}

func (q *QuantizeBeats) reset() {
	f0, fN := q.beats.First.frame, q.beats.Last.frame
	q.nb = q.beats.Last.Subtract(q.beats.First)
	q.df = float64(fN - f0) / float64(q.nb)
	q.Error = nil
}

func (score *Score) beatQuantizer(selxn chan interface{}, beats chan interface{}, apply chan chan bool, calc chan chan QuantizeBeats) {
	var q QuantizeBeats
	for {
		select {
		case ev := <-beats:
			if _, ok := ev.(BeatChanged); ok && !q.Nop() {
				q.reset()
			}
		case ev := <-selxn:
			switch e := ev.(type) {
			case BeatRange:
				q.beats = e
				q.reset()
			default:
				q.beats = BeatRange{nil, nil}
			}
		case reply := <-apply:
			if q.Nop() {
				reply <- true
				continue
			}
			f0 := q.beats.First.frame
			b := q.beats.First.LNext()
			for i := 1; i < q.nb; i++ {
				b.frame = f0 + FrameN(float64(i) * q.df)
				b = b.LNext()
			}
			*q.Error = 0
			score.plumb.C <- BeatChanged{}
			reply <- true
		case reply := <-calc:
			if q.Nop() {
				reply <- q
				continue
			}
			if q.Error == nil {
				q.Error = new(FrameN)
				f0 := q.beats.First.frame
				b := q.beats.First.LNext()
				for i := 1; i < q.nb; i++ {
					qf := f0 + FrameN(float64(i) * q.df)
					ef := FrameN(int64(math.Abs(float64(qf - b.frame))))
					if ef > *q.Error {
						*q.Error = ef
					}
					b = b.LNext()
				}
			}
			reply <- q
		}
	}
}

/* XXX selxn should be a plumb.Port */
func (score *Score) InitQuantizer(selxn chan interface{}) {
	beats := make(chan interface{})
	score.plumb.Sub(score, beats)
	score.quantApply = make(chan chan bool)
	score.quantCalc = make(chan chan QuantizeBeats)
	go score.beatQuantizer(selxn, beats, score.quantApply, score.quantCalc)
}

func (score *Score) QuantizeBeatStat() QuantizeBeats {
	if score.quantCalc == nil {
		return QuantizeBeats{}
	}
	c := make(chan QuantizeBeats)
	score.quantCalc <- c
	return <-c
}

func (score *Score) QuantizeBeats() {
	c := make(chan bool)
	score.quantApply <- c
	<-c
}
