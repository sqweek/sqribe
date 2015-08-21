package score

import (
	"math/big"

	"sqweek.net/sqribe/plumb"

	. "sqweek.net/sqribe/core/types"
)

type BeatPoint interface {
	Beat() *BeatRef
	Offsetf() float64
}

type BeatPt struct {
	beat *BeatRef
	α float64
}

func (pt BeatPt) Beat() *BeatRef {
	return pt.beat
}

func (pt BeatPt) Offsetf() float64 {
	return pt.α
}

type BeatMap interface {
	ToFrame(beat BeatPoint) (FrameN, bool)
	ToBeat(frame FrameN) (BeatPoint, bool)
}

type Score struct {
	BeatList
	staves []*Staff
	beatLen *big.Rat
	plumb *plumb.Port

	quantApply chan chan bool
	quantCalc chan chan QuantizeBeats
}

type Measure struct {
	nbeats int /* length of measure */
	notes []Note /* sorted temporally */
}

func (score *Score) Init(plumb *plumb.Port) {
	score.beatLen = big.NewRat(1, 4)
	score.plumb = plumb
}

func (score *Score) Sub(origin interface{}, c chan interface{}) {
	score.plumb.Sub(origin, c)
}

func (score *Score) Unsub(origin interface{}) {
	score.plumb.Unsub(origin)
}

