package score

import (
	"math"
	"math/big"

	. "github.com/sqweek/sqribe/core/types"
)

type BeatList struct {
	Head, Tail *BeatRef
}

func (l *BeatList) cellp(beat *BeatRef) **BeatRef {
	if beat.prev == nil {
		return &l.Head
	}
	return &beat.prev.next
}

func (l *BeatList) celln(beat *BeatRef) **BeatRef {
	if beat.next == nil {
		return &l.Tail
	}
	return &beat.next.prev
}

func (l *BeatList) Link(beat *BeatRef) {
	*l.cellp(beat) = beat
	*l.celln(beat) = beat
}

func (l *BeatList) Unlink(beat *BeatRef) {
	*l.cellp(beat) = beat.next
	*l.celln(beat) = beat.prev
	beat.next, beat.prev = nil, nil
}

// BeatRange represents the range [First, Last)
type BeatRange struct {
	First, Last *BeatRef
}

func (r BeatRange) MinFrame() FrameN {
	return r.First.frame
}

func (r BeatRange) MaxFrame() FrameN {
	return r.Last.frame - 1
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

func (beat *BeatRef) FrameAtRat(offset *big.Rat) FrameN {
	f, _ := offset.Float64()
	return beat.FrameAt(f)
}

func (beat *BeatRef) FrameAt(offset float64) FrameN {
	next := beat.next
	if next == nil {
		return beat.frame
	}
	return FrameN(float64(beat.frame) * (1 - offset) + float64(next.frame) * (offset))
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
	i, b := 1, beat
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
	return score.update(&MvBeatOp{beat: beat, new: newFrame})
}

type MvBeatOp struct {
	beat *BeatRef
	new, old FrameN
}

func (op *MvBeatOp) apply(score *Score) interface{} {
	op.old = op.beat.frame
	if op.new == op.old {
		return nil
	}
	op.beat.frame = op.new
	return BeatChanged{}
}

func (op *MvBeatOp) undo(score *Score) {
	op.beat.frame = op.old
}

func (beats *BeatList) ToFrame(pt BeatPoint) (FrameN, bool) {
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
func (beats *BeatList) ToBeat(frame FrameN) (BeatPoint, bool) {
	if !beats.HasBeats() || frame < beats.Head.frame || frame > beats.Tail.frame {
		/* should perhaps extrapolate based on bpm... */
		return BeatPt{nil, 0.0}, false
	}
	for b := beats.Head; ; b = b.next {
		if b.next == nil {
			return BeatPt{b, 0.0}, true
		}
		if b.frame <= frame && frame <= b.next.frame {
			α := float64(frame - b.frame) / float64(b.next.frame - b.frame)
			return BeatPt{b, α}, true
		}
	}
}

func (beats *BeatList) BeatFrames() []FrameN {
	f := make([]FrameN, 0)
	for b := beats.Head; b != nil; b = b.next {
		f = append(f, b.frame)
	}
	return f
}

func (beats *BeatList) HasBeats() bool {
	return beats.Head != nil
}

func mkBeats(f []FrameN) (beats BeatList) {
	beats = BeatList{}
	if len(f) == 0 {
		return
	}
	beats.Head = &BeatRef{nil, nil ,f[0]}
	beats.Tail = beats.Head
	for i := 1; i < len(f); i++ {
		b := &BeatRef{beats.Tail, nil, f[i]}
		beats.Tail.next = b
		beats.Tail = b
	}
	return beats
}

func (score *Score) LoadBeats(f []FrameN) {
	score.BeatList = mkBeats(f)
	score.plumb.C <- BeatChanged{}
}

func (score *Score) AddBeat(frame FrameN) {
	score.update(&AddBeatOp{frame: frame})
}

type AddBeatOp struct {
	frame FrameN
	beat *BeatRef
	old FrameN
}

func (op *AddBeatOp) reset() {
	op.old = -1
	op.beat = nil
}

func (op *AddBeatOp) apply(score *Score) interface{} {
	op.reset()
	if score.Head == nil {
		op.beat = &BeatRef{nil, nil, op.frame}
		score.Head = op.beat
		score.Tail = op.beat
		return BeatChanged{}
	}
	tolerance := FrameN(10000) //XXX should be based on sample rate/bpm
	op.beat = score.NearestBeat(op.frame)
	Δf := op.frame - op.beat.frame
	if Δf == 0 {
		return nil
	} else if Δf < -tolerance || Δf > tolerance {
		if Δf > 0 {
			op.beat = &BeatRef{op.beat, op.beat.next, op.frame}
		} else {
			op.beat = &BeatRef{op.beat.prev, op.beat, op.frame}
		}
		score.Link(op.beat)
	} else {
		op.old = op.beat.frame
		op.beat.frame = op.frame
	}
	return BeatChanged{}
}

func (op *AddBeatOp) undo(score *Score) {
	if op.beat == nil {
		return
	} else if op.old != -1 {
		op.beat.frame = op.old
	} else {
		score.Unlink(op.beat)
	}
	op.reset()
}

func (beats *BeatList) NearestBeat(frame FrameN) *BeatRef {
	for b := beats.Head; b != nil; b = b.next {
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

// 2 4 8 16 32 64 128
// 3 6 12 24 48 96
// 5 10 20 40 80
// 7 14 28 56 112
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
			score.update(&QuantizeOp{&q, make(map[*BeatRef]FrameN)})
			if q.Error != nil {
				*q.Error = 0
			}
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

type QuantizeOp struct {
	q *QuantizeBeats
	orig map[*BeatRef]FrameN
}

func (op *QuantizeOp) apply(score *Score) interface{} {
	f0 := op.q.beats.First.frame
	b := op.q.beats.First.LNext()
	for i := 1; i < op.q.nb; i++ {
		op.orig[b] = b.frame
		b.frame = f0 + FrameN(float64(i) * op.q.df)
		b = b.LNext()
	}
	return BeatChanged{}
}

func (op *QuantizeOp) undo(score *Score) {
	for beat, orig_frame := range op.orig {
		beat.frame = orig_frame
		delete(op.orig, beat)
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
