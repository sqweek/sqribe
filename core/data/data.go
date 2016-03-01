package data

import (
	"github.com/sqweek/sqribe/plumb"
)

type Plumbable interface {
	Port() *plumb.Port
}

type Changed struct {
	data interface{}
}

func clip(x, min, max float64) float64 {
	if x < min {
		return min
	} else if x > max {
		return max
	}
	return x
}

type BoundFloat struct {
	val, min, max float64
	plumb *plumb.Port
}

func MkBoundFloat(v, min, max float64, port *plumb.Port) *BoundFloat {
	if port == nil {
		port = plumb.MkPort()
	}
	return &BoundFloat{v, min, max, port}
}

func (float *BoundFloat) Value() float64 {
	return float.val
}

func (float *BoundFloat) Port() *plumb.Port {
	return float.plumb
}

func (float *BoundFloat) Set(f float64) {
	f = clip(f, float.min, float.max)
	if f != float.val {
		float.val = f
		float.plumb.C <- Changed{float}
	}
}

func (float *BoundFloat) Shunt(perc float64) {
	float.Set(float.val + (float.max - float.min) * clip(perc, -1, 1))
}

// Returns a number on [0,1] representing the current value
func (float BoundFloat) Posn() float64 {
	return (float.val - float.min) / (float.max - float.min)
}
