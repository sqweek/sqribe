package main

type Errstr struct {
	str string
}

func (e *Errstr) Error() string {
	return e.str
}
