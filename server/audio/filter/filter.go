package filter

import (
	"math"
)

type Type uint8

const (
	Vaporwave Type = iota
	Nightcore
	Rotation
	Tremolo
	Vibrato
	LowPass
)

type Filters struct {
	Enabled []Type  `json:"enabled,omitempty"`
	Pitch   float64 `json:"pitch,omitempty"`
	Speed   float64 `json:"speed,omitempty"`
}

func (f *Filters) resolvedTimescale() (speed, pitch float64) {
	speed = 1.0
	pitch = 1.0

	for _, ft := range f.Enabled {
		switch ft {
		case Nightcore:
			speed *= 1.3
			pitch *= 1.3
		case Vaporwave:
			speed *= 0.8
			pitch *= 0.8
		}
	}

	if f.Speed > 0 {
		speed *= f.Speed
	}
	if f.Pitch > 0 {
		pitch *= f.Pitch
	}

	return speed, pitch
}

func (f *Filters) hasFilter(ft Type) bool {
	for _, t := range f.Enabled {
		if t == ft {
			return true
		}
	}
	return false
}

func (f *Filters) IsEmpty() bool {
	return f == nil || (len(f.Enabled) == 0 && f.Pitch <= 0 && f.Speed <= 0)
}

type Processor struct {
	filters *Filters

	speed float64
	pitch float64

	tremoloPhase  float64
	vibratoPhase  float64
	rotationPhase float64

	lpPrevL float64
	lpPrevR float64

	vibratoBuf []int16

	sampleRate float64
}

func NewProcessor(filters *Filters, sampleRate float64) *Processor {
	speed, pitch := filters.resolvedTimescale()
	p := &Processor{
		filters:    filters,
		speed:      speed,
		pitch:      pitch,
		sampleRate: sampleRate,
	}
	if filters.hasFilter(Vibrato) {
		p.vibratoBuf = make([]int16, 960*2)
	}
	return p
}

func (p *Processor) TimescaleRatio() float64 {
	return p.speed
}

func (p *Processor) PitchRatio() float64 {
	return p.pitch
}

func (p *Processor) Process(samples []int16) {
	n := len(samples) / 2

	if p.filters.hasFilter(Tremolo) {
		const freq = 4.0
		const depth = 0.6
		phaseInc := 2.0 * math.Pi * freq / p.sampleRate

		for i := range n {
			mod := 1.0 - depth*0.5*(1.0+math.Sin(p.tremoloPhase))
			l := float64(samples[i*2]) * mod
			r := float64(samples[i*2+1]) * mod
			samples[i*2] = clampInt16(l)
			samples[i*2+1] = clampInt16(r)
			p.tremoloPhase += phaseInc
		}
		p.tremoloPhase = math.Mod(p.tremoloPhase, 2*math.Pi)
	}

	if p.filters.hasFilter(Vibrato) {
		const freq = 4.0
		const depth = 0.5
		const maxDelay = 0.002
		phaseInc := 2.0 * math.Pi * freq / p.sampleRate

		buf := p.vibratoBuf
		if len(buf) < len(samples) {
			buf = make([]int16, len(samples))
			p.vibratoBuf = buf
		}
		copy(buf[:len(samples)], samples)

		for i := range n {
			delaySamples := maxDelay * depth * (0.5 + 0.5*math.Sin(p.vibratoPhase)) * p.sampleRate
			srcIdx := float64(i) - delaySamples
			if srcIdx < 0 {
				srcIdx = 0
			}

			idx0 := int(srcIdx)
			frac := srcIdx - float64(idx0)
			if idx0 >= n-1 {
				idx0 = n - 2
				frac = 1.0
			}
			if idx0 < 0 {
				idx0 = 0
				frac = 0
			}

			for ch := range 2 {
				s0 := float64(buf[idx0*2+ch])
				s1 := float64(buf[(idx0+1)*2+ch])
				samples[i*2+ch] = clampInt16(s0 + (s1-s0)*frac)
			}

			p.vibratoPhase += phaseInc
		}
		p.vibratoPhase = math.Mod(p.vibratoPhase, 2*math.Pi)
	}

	if p.filters.hasFilter(Rotation) {
		const rotationHz = 0.2
		phaseInc := 2.0 * math.Pi * rotationHz / p.sampleRate

		for i := range n {
			pan := math.Sin(p.rotationPhase)
			lGain := math.Cos((pan + 1.0) * math.Pi / 4.0)
			rGain := math.Sin((pan + 1.0) * math.Pi / 4.0)

			l := float64(samples[i*2])
			r := float64(samples[i*2+1])
			mono := (l + r) * 0.5
			samples[i*2] = clampInt16(mono * lGain * math.Sqrt2)
			samples[i*2+1] = clampInt16(mono * rGain * math.Sqrt2)

			p.rotationPhase += phaseInc
		}
		p.rotationPhase = math.Mod(p.rotationPhase, 2*math.Pi)
	}

	if p.filters.hasFilter(LowPass) {
		const smoothing = 20.0
		coeff := 1.0 / smoothing

		for i := range n {
			sL := float64(samples[i*2])
			sR := float64(samples[i*2+1])

			p.lpPrevL += (sL - p.lpPrevL) * coeff
			p.lpPrevR += (sR - p.lpPrevR) * coeff

			samples[i*2] = clampInt16(p.lpPrevL)
			samples[i*2+1] = clampInt16(p.lpPrevR)
		}
	}
}

func clampInt16(v float64) int16 {
	if v > 32767 {
		return 32767
	}
	if v < -32768 {
		return -32768
	}
	return int16(v)
}
