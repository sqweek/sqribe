package main

import (
	"math"
)

func mod(n, d int) int {
	r := n % d
	if r < 0 {
		return d + r
	}
	return r
}

func ceil(n, d int) int {
	q, r := n / d, n % d
	if r == 0 {
		return q
	} else if n < 0 {
		return q
	}
	return q + 1
}

/* divides n by d, rounding away from zero. assumes d is positive */
func divØ(n, d int) int {
	q, r := n / d, n % d
	if r == 0 {
		return q
	} else if n < 0 {
		return q - 1
	}
	return q + 1
}

func slog(s int16) float64 {
	return float64(s)
	if s == 0 {
		return 0.0
	} else if s < 0 {
		return -math.Log(float64(-s))
	} else {
		return math.Log(float64(s))
	}
}

func snapto(x, origin, step int) int {
	d := x - origin
	var sgn int
	if (d < 0) {
		sgn = -1
	} else {
		sgn = 1
	}
	rem := (sgn * d) % step
	if rem < step/2 {
		return x - sgn * rem
	}
	return x + sgn * (step - rem)
}

func clip(x, min, max float64) float64 {
	if x < min {
		return min
	} else if x > max {
		return max
	}
	return x
}
