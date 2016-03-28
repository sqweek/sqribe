package types

type FrameN int64 // frame index or frame count
type SampleN uint64 // sample index or sample count

/* MinFrame() and MaxFrame() are both inclusive.
 * Min is assumed to always be <= Max.
 * if Min >= Max, playback treats the range as empty. */
type TimeRange interface {
	MinFrame() FrameN
	MaxFrame() FrameN
}

type FrameRange struct {
	Min, Max FrameN
}

func (r FrameRange) MinFrame() FrameN {
	return r.Min
}

func (r FrameRange) MaxFrame() FrameN {
	return r.Max
}
