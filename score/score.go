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

type ScoreOp interface {
	apply(score *Score) bool
}

type request struct {
	op ScoreOp
	result chan bool
}

type Score struct {
	BeatList
	staves []*Staff
	beatLen *big.Rat
	plumb *plumb.Port

	updates chan request

	quantApply chan chan bool
	quantCalc chan chan QuantizeBeats
}

type Measure struct {
	nbeats int /* length of measure */
	notes []Note /* sorted temporally */
}

func MkScore(plumb *plumb.Port) *Score {
	score := Score {
		beatLen: big.NewRat(1, 4),
		plumb: plumb,
		updates: make(chan request),
	}
	go func() {
		for req := range score.updates {
			req.result <- req.op.apply(&score)
		}
	}()
	return &score
}

func (score *Score) Close() {
	close(score.updates)
}

func (score *Score) IsEmpty() bool {
	return len(score.staves) == 0 && score.Head == nil
}

func (score *Score) Sub(origin interface{}, c chan interface{}) {
	score.plumb.Sub(origin, c)
}

func (score *Score) Unsub(origin interface{}) {
	score.plumb.Unsub(origin)
}

func (score *Score) update(op ScoreOp) bool {
	req := request{op, make(chan bool)}
	score.updates <- req
	return <-req.result
}

