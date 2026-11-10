package main

import "time"

type pose struct {
	key, title string
	stride     float64
}

var poses = []pose{
	{"resting", "趴着", 0}, {"sitting", "坐着侧耳", 0}, {"watching", "抬头张望", 0},
	{"grooming", "低头梳毛", 0}, {"belly", "躺着伸爪", 0}, {"tail", "背身摆尾", 0},
	{"sleeping", "侧躺睡觉", 0}, {"walking", "一直慢慢走", 96},
	{"sprinting", "小小冲刺", 210}, {"spinning", "俯视转圈", 0}, {"scaling", "变大变小", 0},
}

const (
	resting  = 0
	sitting  = 1
	sleeping = 6
)

var active = []int{1, 2, 0, 3, 5, 4, 9, 10}
var sizes = []int{120, 180, 240, 300, 360}

type patrol struct {
	x         float64
	direction float64
	restUntil time.Time
}

// advance returns true when the cat stands up and a new gait cycle should begin.
func (p *patrol) advance(now time.Time, elapsed, speed, left, right float64) bool {
	if left >= right {
		return false
	}
	if !p.restUntil.IsZero() {
		if now.Before(p.restUntil) {
			return false
		}
		p.restUntil = time.Time{}
		p.direction = -p.direction
		return true
	}
	next := p.x + elapsed*speed*p.direction
	if (next <= left && p.direction < 0) || (next >= right && p.direction > 0) {
		p.restUntil = now.Add(2 * time.Second)
	}
	p.x = min(right, max(left, next))
	return false
}
