package score

import (
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
	index int
	frame FrameN
}

type BeatChanged struct {
}

func (beat *BeatRef) Frame() FrameN {
	return beat.frame
}

func (beat *BeatRef) Prev(score *Score) *BeatRef {
	score.RLock()
	defer score.RUnlock()
	if beat.index - 1 < 0 {
		return beat
	}
	return score.beats[beat.index - 1]
}

func (beat *BeatRef) Next(score *Score) *BeatRef {
	score.RLock()
	defer score.RUnlock()
	if beat.index + 1 >= len(score.beats) {
		return beat
	}
	return score.beats[beat.index + 1]
}

func (score *Score) Shunt(br BeatRange, Δbeat int) BeatRange {
	score.RLock()
	defer score.RUnlock()
	b0 := score.ClipBeat(br.First.index + Δbeat)
	bN := score.ClipBeat(br.Last.index + Δbeat)
	return BeatRange{score.beats[b0], score.beats[bN]}
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

func (score *Score) ToFrame(beat BeatPoint) (FrameN, bool) {
	score.RLock()
	defer score.RUnlock()
	b := beat.Beat()
	b2 := b.Next(score)
	α := beat.Offsetf()
	if (α < 1e-6 && b2 == b) {
		return b.frame, true
	}
	if b2 != b {
		return FrameN((1 - α) * float64(b.frame) + α * float64(b2.frame)), true
	}
	return -1, false
}

/* returns insertion index and true if the frame is already present */
func (score *Score) index(frame FrameN) (int, bool) {
	/* TODO binary search instead of linear */
	if len(score.beats) == 0 || frame < score.beats[0].frame {
		return 0, false
	}
	for i := 0; i < len(score.beats); i++ {
		if frame == score.beats[i].frame {
			return i, true
		} else if i + 1 >= len(score.beats) || frame < score.beats[i+1].frame {
			return i+1, false
		}
	}
	return len(score.beats), false
}

/* returns a fractional beat, and true if it is within the defined beat range */
func (score *Score) ToBeat(frame FrameN) (BeatPoint, bool) {
	score.RLock()
	defer score.RUnlock()
	if len(score.beats) == 0 || frame < score.beats[0].frame || frame > score.beats[len(score.beats)-1].frame {
		/* should perhaps extrapolate based on bpm... */
		return Beatf{score, -1}, false
	}
	i, exact := score.index(frame)
	if exact {
		return Beatf{score, float64(i)}, true
	}
	α := float64(frame - score.beats[i-1].frame) / float64(score.beats[i].frame - score.beats[i-1].frame)
	return Beatf{score, float64(i-1) + α}, true
}

func (score *Score) ClipBeat(index int) int {
	if len(score.beats) == 0 {
		return -1
	}
	score.RLock()
	defer score.RUnlock()
	if index > len(score.beats) {
		return len(score.beats) - 1
	} else if index < 0 {
		return 0
	}
	return index
}

func (score *Score) Beats() []*BeatRef {
	return score.beats
}

func (score *Score) BeatFrames() []FrameN {
	score.RLock()
	defer score.RUnlock()
	f := make([]FrameN, 0, len(score.beats))
	for i := 0; i < len(score.beats); i++ {
		f = append(f, score.beats[i].frame)
	}
	return f
}

func newBeat(index int, frame FrameN) *BeatRef {
	beat := new(BeatRef)
	beat.index = index
	beat.frame = frame
	return beat
}

func (score *Score) LoadBeats(f []FrameN) {
	score.Lock()
	if len(score.beats) > 0 {
		score.beats = score.beats[0:0]
	}
	for i := 0; i < len(f); i++ {
		score.beats = append(score.beats, newBeat(i, f[i]))
	}
	score.Unlock()
	score.plumb.C <- BeatChanged{}
}

func (score *Score) AddBeat(frame FrameN) {
	score.Lock()
	changed := score.addBeat(frame)
	score.Unlock()
	if changed {
		score.plumb.C <- BeatChanged{}
	}
}

func (score *Score) addBeat(frame FrameN) bool {
	if len(score.beats) == 0 {
		score.beats = append(score.beats, newBeat(0, frame))
		return true
	}
	tolerance := FrameN(10000) //XXX should be based on sample rate/bpm
	i, exact := score.index(frame)
	if exact {
		return false
	}
	if i > 0 && frame - score.beats[i-1].frame < tolerance {
		score.beats[i-1].frame = (score.beats[i-1].frame + frame) / 2
	} else if i == len(score.beats) {
		score.beats = append(score.beats, newBeat(len(score.beats), frame))
	} else if score.beats[i].frame - frame < tolerance {
		score.beats[i].frame = (score.beats[i].frame + frame) / 2
	} else {
		score.beats = append(score.beats, nil)
		copy(score.beats[i+1:], score.beats[i:])
		score.beats[i] = newBeat(i, frame)
		for j := i+1; j < len(score.beats); j++ {
			score.beats[j].index = j
		}
	}
	return true
}

func (score *Score) NearestBeat(frame FrameN) *BeatRef {
	score.RLock()
	defer score.RUnlock()
	if len(score.beats) == 0 {
		return nil
	}
	i, exact := score.index(frame)
	if exact || i == 0 {
		return score.beats[i]
	} else if i == len(score.beats) {
		return score.beats[len(score.beats) - 1]
	} else if frame - score.beats[i-1].frame < score.beats[i].frame - frame {
		return score.beats[i - 1]
	} else {
		return score.beats[i]
	}
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
		b = b.Next(score)
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

func (q *QuantizeBeats) reset(score *Score) {
	f0, fN := q.beats.First.frame, q.beats.Last.frame
	q.nb = 0
	for b := q.beats.First; b != q.beats.Last; b = b.Next(score) {
		q.nb++
	}
	q.df = float64(fN - f0) / float64(q.nb)
	q.Error = nil
}

func (score *Score) beatQuantizer(selxn chan interface{}, beats chan interface{}, apply chan chan bool, calc chan chan QuantizeBeats) {
	var q QuantizeBeats
	for {
		select {
		case ev := <-beats:
			if _, ok := ev.(BeatChanged); ok && !q.Nop() {
				q.reset(score)
			}
		case ev := <-selxn:
			if len(score.beats) > 0 {
				switch e := ev.(type) {
				case BeatRange:
					q.beats = e
					q.reset(score)
				default:
					q.beats = BeatRange{nil, nil}
				}
			}
		case reply := <-apply:
			if q.Nop() {
				reply <- true
				continue
			}
			f0 := q.beats.First.frame
			b := q.beats.First.Next(score)
			for i := 1; i < q.nb; i++ {
				b.frame = f0 + FrameN(float64(i) * q.df)
				b = b.Next(score)
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
				b := q.beats.First.Next(score)
				for i := 1; i < q.nb; i++ {
					qf := f0 + FrameN(float64(i) * q.df)
					ef := FrameN(int64(math.Abs(float64(qf - b.frame))))
					if ef > *q.Error {
						*q.Error = ef
					}
					b = b.Next(score)
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
