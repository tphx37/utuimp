package tui

import "math"

// The visualizer's scenes. Each one fills v.fb; see visualizer.go for what
// happens to it next.
//
// In every scene each element listens to a voice of its own (see voiceBass
// and the others): the comment of the scene says which.

// fmod1 is math.Mod(x, 1), the same to the bit: x less its whole part,
// with x's sign. math.Mod takes a slow general path, and the scenes ask for
// it several times a pixel (it was a quarter of the tunnel's frame).
func fmod1(x float64) float64 { return x - math.Trunc(x) }

// fmodN is math.Mod(x, n) the same way; to the bit for n a power of two.
func fmodN(x, n float64) float64 { return x - n*math.Trunc(x/n) }

// tunnelSparks is hash1 of each strip of the tunnel's wall, -71 to 71 (a
// strip is a whole number, and its sign is that of the angle): which have a
// spark is worked out once, not with a sine every pixel.
var tunnelSparks = func() (h [143]float64) { //nolint:gochecknoglobals // a fixed table
	for i := range h {
		h[i] = hash1(float64(i - 71))
	}
	return h
}()

// fsin is math.Sin to within a ten-millionth, which no colour can show, at
// a fraction of its cost: the angle is brought within a quarter turn of
// nothing and a short series does the rest. The tunnel takes a sine or two
// for every pixel, and math.Sin was a seventh of its frame.
func fsin(x float64) float64 {
	k := math.Floor(x/math.Pi + 0.5)
	r := x - k*math.Pi // within ±π/2; sin x is ± sin r
	r2 := r * r
	s := r * (1 + r2*(-1.0/6+r2*(1.0/120+r2*(-1.0/5040+r2*(1.0/362880+r2*(-1.0/39916800))))))
	if int64(k)&1 != 0 {
		return -s
	}
	return s
}

// expNeg is e^(-q) for q ≥ 0, read off a table with a straight line between
// its steps (a 256th apart) up to expNegMax, to two millionths of the value:
// far below what a colour shows, at a fraction of the cost of math.Exp,
// which several glows of a scene take for every pixel.
func expNeg(q float64) float64 {
	if q >= expNegMax || q < 0 {
		return math.Exp(-q)
	}
	x := q * expNegStep
	i := int(x)
	return expNegLUT[i] + (expNegLUT[i+1]-expNegLUT[i])*(x-float64(i))
}

const (
	expNegMax  = 24.0
	expNegStep = 256.0
)

var expNegLUT = func() (t [int(expNegMax*expNegStep) + 2]float64) { //nolint:gochecknoglobals // a fixed table
	for i := range t {
		t[i] = math.Exp(-float64(i) / expNegStep)
	}
	return t
}()

// drawTunnel: flying down a neon tube.
//
//	rings          bass (they thicken and brighten)
//	strip edges    body
//	glow far ahead vocal
//	wall panels    the spectrum, bass at the top and bottom
//	sparks         air
//	the squeeze    the drum
//
// In words: The walls are split into strips, each
// lit by one band (bass at the top and bottom, treble at the sides); rings
// rush past faster when the music is loud, sparks streak along the walls
// with the treble, and every beat sends a wave of light down the tube.
func (v *visualizer) drawTunnel() {
	w := v.w
	sway := 0.6 + 0.8*v.drive
	cu, cv := 0.18*sway*fsin(v.t*0.53), 0.12*sway*math.Cos(v.t*0.41)
	rot := v.t*0.12 + v.twirl
	core := v.pal[2].mix(vizWhite, 0.5)
	waveZ, waveA := 0.3+v.bigT*11, v.bigS*expNeg(v.bigT*2.2)
	// The tube is round only at rest: the drum squeezes it into lobes.
	const lobes = 3.0
	squeeze := 0.03*v.drive + 0.22*v.kick
	// The squeeze is sin(3θ + φ) round the tube, θ the angle of the pixel:
	// the imaginary part of e^(iφ)·(x+iy)³/r³, with no sine nor angle to
	// work out for it.
	phi := 3*math.Pi + 6*math.Pi*rot + v.t*1.5
	sinP, cosP := math.Sin(phi), math.Cos(phi)
	par(v.h2, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := range w {
				px, py := v.unit(x, y)
				px, py = px-cu, py-cv
				r0 := math.Sqrt(px*px + py*py)
				r := r0 + 1e-4
				a := math.Atan2(py, px)/(2*math.Pi) + 0.5
				if squeeze != 0 && r0 > 0 {
					// (px + i·py)³ = re + i·im
					re, im := px*(px*px-3*py*py), py*(3*px*px-py*py)
					r *= 1 + squeeze*(im*cosP+re*sinP)/(r0*r0*r0)
				} else if squeeze != 0 {
					r *= 1 + squeeze*fsin((a+rot)*2*math.Pi*lobes+v.t*1.5)
				}
				z := 0.55 / r
				sa := a + rot + z*0.05
				d := z*3 + v.t*2.4
				mirror := math.Abs(fmodN(sa*2+4, 2) - 1) // 0..1 around the tube, twice
				band := v.bandAt(mirror)
				ring := 0.0
				if q := sq(fmod1(d)-0.5) * (90 - 55*v.voice[voiceBass]); q < 16 {
					ring = expNeg(q)
				}
				strip := fmod1(sa*24 + 48)
				edge := expNeg(sq(strip-0.5) * 70)
				panel := fmod1(d * 0.5)
				lit := band * band * smooth(0.25, 0.9, panel+band*0.4) * (1 - smooth(0.8, 1, panel))
				bright := ring*(0.2+1.5*v.voice[voiceBass]+0.5*v.pulse[voiceBass]) + edge*(0.05+0.5*v.voice[voiceBody]) + lit*0.7
				// A spark on some strips: a bright head with a tail behind it.
				cell := sa*72 + 720
				if hsh := tunnelSparks[int(fmodN(math.Floor(cell), 72))+71]; hsh > 0.72 {
					f := fmod1(d*0.3 + hsh*31)
					across := sq(fsin((cell - math.Floor(cell)) * math.Pi))
					bright += expNeg(f*9) * sq(across) * (0.1 + 1.6*v.voice[voiceAir] + 2*v.pulse[voiceAir])
				}
				fog := sq(clamp01(r * 1.6))
				c := v.palAt(d*0.05 + mirror*0.9).mul(bright * fog * (0.95 + v.beat*0.15))
				// The two glows only where they are more than a
				// ten-millionth of themselves (e^-16): the rest of the
				// screen is spared their e^x.
				if q := sq(z-waveZ) * 1.1; q < 16 && waveA > 0 {
					c = c.add(core.mul(expNeg(q) * waveA * 0.45 * fog))
				}
				if q := r * r * (26 - 14*v.voice[voiceVocal]); q < 16 {
					c = c.add(core.mul(expNeg(q) * (0.1 + 0.7*v.voice[voiceVocal])))
				}
				v.fb[y*w+x] = c
			}
		}
	})
}

// drawTerrain: gliding down a canyon whose floor is the spectrum.
//
//	the floor      the spectrum, bass in the middle
//	the sun        bass (its size), vocal (its halo)
//	canyon walls   body
//	grid lines     bright
//	stars          air
//
// In words: Each new
// spectrum row rises up in front of the camera and rolls away to the
// horizon, under the stars and a pulsing sun.
func (v *visualizer) drawTerrain() {
	w, h2 := v.w, v.h2
	fh := float64(h2)
	horizon := fh * 0.4
	proj := fh * 0.5
	const camH, zNear, zFar, width = 1.7, 1.0, 26.0, 7.0
	rowLen := (zFar - zNear) / vizHistory
	scroll := (float64(v.histN) + v.histFrac) * rowLen

	// Sky: dark at the top, glowing towards the horizon, with a sun.
	sunX, sunY := float64(w)/2, horizon-fh*0.1
	sunR := fh * (0.1 + 0.09*v.voice[voiceBass] + 0.03*v.pulse[voiceBass])
	sunW := 1 + 0.35*v.kick // the drum squashes the sun flat
	haze := v.pal[0].mul(0.1)
	par(h2, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			fy := float64(y)
			if fy >= horizon {
				for x := range w {
					v.fb[y*w+x] = haze
				}
				continue
			}
			k := clamp01(fy / horizon)
			sky := v.pal[0].mul(0.02 + 0.22*k*k*k)
			for x := range w {
				c := sky
				if hsh := hash1(float64(x)*1.37 + float64(y)*71.3); hsh > 0.988 {
					tw := 0.55 + 0.45*math.Sin(v.t*2.5+hsh*900)
					c = c.add(vizWhite.mix(v.pal[2], 0.3).mul((hsh - 0.988) / 0.012 * tw * (1 - k) * (0.2 + 1.6*v.voice[voiceAir] + 2*v.pulse[voiceAir])))
				}
				dx, dy := (float64(x)+0.5-sunX)/v.ya/sunW, (fy-sunY)*sunW
				if r := math.Sqrt(dx*dx+dy*dy) / sunR; r < 1.8 {
					band := fmod1((fy-sunY)/fh*14 + v.t*0.4)
					if r < 1 && (dy <= 0 || band >= 0.18+dy/sunR*0.3) {
						c = v.pal[1].mix(v.pal[2], clamp01(dy/sunR*0.5+0.5)).mul(1.15)
					} else {
						c = c.add(v.pal[1].mul((0.15 + 0.7*v.voice[voiceVocal]) * expNeg(math.Max(0, r-1)*(5-2.5*v.voice[voiceVocal]))))
					}
				}
				v.fb[y*w+x] = c
			}
		}
	})

	par(w, func(x0, x1 int) {
		for x := x0; x < x1; x++ {
			lx := (float64(x) + 0.5 - float64(w)/2) / (float64(w) / 2) * 1.6
			yb := fh
			for z := zNear; z < zFar; z += 0.012*z + 0.02 {
				wx := lx * z
				hz := (z - zNear) / rowLen
				bx := math.Abs(wx) / width
				hgt := 0.0
				if bx < 1 {
					hgt = histAt(v.hist, bx, hz-v.histFrac) * (0.4 + 0.9*smooth(0, 0.45, bx))
					hgt *= 0.55 + 0.7*v.drive + 1.1*v.kick*expNeg(z*0.25) // the ground near you jumps with the drum
				}
				// The canyon's walls, beyond the spectrum.
				zs := z + scroll
				hgt += smooth(0.8, 1.6, bx) * (0.55 + 0.7*v.voice[voiceBody]) * (0.95 + 0.4*math.Sin(zs*0.8+lx*3) + 0.25*math.Sin(zs*2.3))
				sy := horizon + (camH-hgt)*proj/z
				if sy >= yb {
					continue
				}
				fog := expNeg(z * 0.085)
				gx := math.Abs(fmod1(wx*1.3+100) - 0.5)
				gz := math.Abs(fmod1(zs*0.7+100) - 0.5)
				grid := math.Max(expNeg(sq(0.5-gx)*400/(z*0.3+1)), expNeg(sq(0.5-gz)*300/(z*0.3+1)))
				grid *= expNeg(z * 0.09)                                 // far lines would blur into a haze
				lit := math.Min(hgt, 1) * (1 - 0.6*smooth(0.8, 1.6, bx)) // the walls stay in the dark
				base := v.palAt(hgt*0.9 + 0.2).mul(0.03 + lit*0.35)
				line := v.palAt(hgt*0.9 + 1.2).mul(grid * (0.25 + lit*1.1 + 0.9*v.voice[voiceBright]))
				top := int(math.Max(0, sy))
				for y := top; y < int(yb); y++ {
					c := base.add(line)
					if y == top && hgt > 0.1 {
						c = c.add(v.palAt(hgt + 1.5).mul(lit * 1.1)) // the lit crest
					}
					v.fb[y*w+x] = c.mul(fog).add(haze.mul(1 - fog))
				}
				yb = sy
				if yb <= 0 {
					break
				}
			}
		}
	})
}

// histAt samples the spectrum history at band x (0..1) and row r.
func histAt(hist [][]float64, x, r float64) float64 {
	r = math.Max(0, r)
	i := int(r)
	if i >= len(hist)-1 {
		return lerpRow(hist[len(hist)-1], x)
	}
	a, b := lerpRow(hist[i], x), lerpRow(hist[i+1], x)
	return a + (b-a)*(r-float64(i))
}

// drawNebula:
//
//	the ring's size      bass
//	its waveform         vocal (how far it swings)
//	its petals           the drum
//	the crown inside     the spectrum
//	how fast it all flows away, and turns   body
//	sparks               air
//
// In words: the waveform bent into a ring, and everything it ever drew
// flowing away from it: each frame the picture before is pushed outwards,
// turned and twisted a little, so the ring leaves clouds behind. Beats
// throw sparks.
func (v *visualizer) drawNebula(dt float64) {
	f := dt * 30
	fade := math.Pow(0.8+0.04*v.level, f)
	zoom := 1 - (0.025+0.035*v.voice[voiceBody]+0.03*v.kick)*f*v.strength
	turn := 1.0
	if v.beatN/32%2 == 1 {
		turn = -1 // every 32 beats the clouds turn the other way
	}
	rot := (0.008 + 0.03*v.voice[voiceBody] + 0.03*v.beat) * turn * f * v.strength
	twist := 0.035 * math.Sin(v.t*0.31) * f
	v.warp(fade, func(u, w float64) (float64, float64) {
		r := math.Sqrt(u*u + w*w)
		s, c := math.Sincos(rot + twist*math.Sin(r*4-v.t*1.3))
		k := zoom + 0.006*f*math.Sin(r*7-v.t*2)
		return (u*c - w*s) * k, (u*s + w*c) * k
	})

	// The ring: the waveform twice, mirrored, so it closes.
	const n = 2 * vizWave
	spin := v.t*0.25 + v.twirl
	// The drum pushes the ring out into petals.
	const petals = 3.0
	bulge := 0.02*v.drive + 0.13*v.kick
	r0 := 0.24 + 0.16*v.voice[voiceBass] + 0.05*v.pulse[voiceBass]
	gain := math.Sqrt(v.dense()) // thin lines on a big screen need more light
	var lx, ly float64
	for i := range n + 1 {
		j := i % n
		if j >= vizWave {
			j = n - 1 - j
		}
		a := float64(i)/n*2*math.Pi + spin
		r := r0 + (0.05+0.22*v.voice[voiceVocal])*v.wave[j] + bulge*math.Sin(a*petals)
		px, py := v.pix(math.Cos(a)*r, math.Sin(a)*r)
		if i > 0 {
			c := v.palAt(float64(i)/n*3 + v.t*0.12).mul((0.05 + 0.07*v.level) * gain)
			v.line(lx, ly, px, py, c)
		}
		lx, ly = px, py
	}
	// The spectrum as a small crown inside it.
	for i := range 96 {
		x := float64(i) / 96
		band := v.bandAt(math.Abs(x*2 - 1))
		a := x*2*math.Pi - spin*1.7
		ca, sa := math.Cos(a), math.Sin(a)
		x0, y0 := v.pix(ca*0.07, sa*0.07)
		x1, y1 := v.pix(ca*(0.07+band*0.16), sa*(0.07+band*0.16))
		v.line(x0, y0, x1, y1, v.palAt(x*3+1+v.t*0.1).mul(0.02+band*0.06))
	}
	// Sparks.
	if v.onBeat {
		for range 90 {
			a := v.rnd() * 2 * math.Pi
			sp := 0.7 + v.rnd()*1.8
			v.parts = append(v.parts, vizPart{
				x: math.Cos(a) * r0, y: math.Sin(a) * r0,
				vx: math.Cos(a) * sp, vy: math.Sin(a) * sp,
				life: 1, hue: v.rnd() * 3,
			})
		}
	}
	v.stepParts(dt, 1.3, 0.6)
	for i := range v.parts {
		p := &v.parts[i]
		px, py := v.pix(p.x, p.y)
		v.drawPart(p, px, py, v.palAt(p.hue).mix(vizWhite, 0.35).mul(p.life*0.9*gain))
	}
}

// dense is how much brighter single points must be drawn so that a cloud
// of them looks the same on a big screen, where they lie further apart, as
// on a small one.
func (v *visualizer) dense() float64 {
	return math.Max(0.7, math.Min(2.5, v.sc/30))
}

// stepParts moves the sparks; they live for lifetime seconds and slow down
// by drag.
// drawPart draws the particle p, now at (px, py): the way it went since
// the last frame (see streak), or a point the first time.
func (v *visualizer) drawPart(p *vizPart, px, py float64, c vcol) {
	if p.seen && math.Hypot(px-p.sx, py-p.sy) < float64(v.w)/4 {
		v.streak(p.sx, p.sy, px, py, c)
	} else {
		v.splat(px, py, c)
	}
	p.sx, p.sy, p.seen = px, py, true
}

func (v *visualizer) stepParts(dt, lifetime, drag float64) {
	k := expNeg(dt * drag)
	live := v.parts[:0]
	for _, p := range v.parts {
		p.life -= dt / lifetime
		if p.life <= 0 {
			continue
		}
		p.x, p.y, p.z = p.x+p.vx*dt, p.y+p.vy*dt, p.z+p.vz*dt
		p.vx, p.vy, p.vz = p.vx*k, p.vy*k, p.vz*k
		live = append(live, p)
	}
	v.parts = live
	if len(v.parts) > 1500 {
		v.parts = v.parts[len(v.parts)-1500:]
	}
}

// drawSpectrum:
//
//	the bars            the spectrum, bass in the middle
//	the light off the floor   bass
//	the waveform above  vocal (how far it swings)
//	the peak marks      air (how bright)
//	the floor's wave    the drum
//
// In words: the classic one, done properly. Bars grow from the middle
// out (bass in the centre), each with a falling peak mark, standing on a
// floor that mirrors them; the waveform runs across the sky above.
func (v *visualizer) drawSpectrum() {
	w, h2 := v.w, v.h2
	fh := float64(h2)
	floor := int(fh * 0.68)
	ff := float64(floor)
	nb := min(vizBands, max(6, w/4)) // bars on each side
	bw := float64(w) / 2 / float64(nb)
	par(w, func(x0, x1 int) {
		for x := x0; x < x1; x++ {
			fx := math.Abs(float64(x)+0.5-float64(w)/2) / (float64(w) / 2)
			pos := fx * float64(nb)
			bi := min(nb-1, int(pos))
			at := (float64(bi) + 0.5) / float64(nb)
			gap := bw >= 3 && pos-float64(bi) > 1-1/bw
			val := lerpRow(v.bands, at)
			barH := val * ff * (0.62 + 0.2*v.drive + 0.2*v.kick)
			peakY := lerpRow(v.peaks, at) * ff * 0.8
			hue := fx*1.5 + v.t*0.06
			glow := v.pal[1].mul(0.02 + 0.2*v.voice[voiceBass])
			// The floor is flat only at rest: the drum sends a wave along it,
			// and the bars ride the wave.
			lift := fh * 0.05 * v.kick * math.Sin(fx*math.Pi*2-v.t*5)
			for y := range h2 {
				// The backdrop: light rising from the floor.
				fy := float64(y) + lift
				d := math.Abs(fy-ff) / fh
				c := v.pal[0].mul(0.015).add(glow.mul(expNeg(d*5) * (1 - fx*0.6)))
				hgt := ff - fy // above the floor
				dim := 1.0
				if hgt < 0 {
					// The mirror: squashed, and fading with depth.
					dim = 0.4 * math.Exp(hgt/(fh*0.09))
					hgt = -hgt * 1.7
				}
				if !gap {
					switch {
					case hgt < barH:
						k := hgt / (ff * 0.8)
						c = c.add(v.palAt(hue + k*0.9).mul((0.32 + 1.1*k + 0.1*v.beat) * dim))
						if barH-hgt < 1 {
							c = c.add(vizWhite.mul(0.35 * dim)) // the bar's bright top
						}
					case math.Abs(hgt-peakY-1) < 0.75 && peakY > 1:
						c = c.add(v.palAt(hue+1.2).mix(vizWhite, 0.5).mul(0.9 * dim))
					}
				}
				if math.Abs(fy-ff) < 0.5 {
					c = c.add(v.pal[2].mul(0.3 + 0.2*v.beat))
				}
				v.fb[y*w+x] = c
			}
		}
	})
	// The waveform.
	mid, amp := fh*0.13, fh*(0.03+0.12*v.voice[voiceVocal])
	col := v.pal[2].mix(vizWhite, 0.4).mul(0.3 + 0.7*v.voice[voiceVocal])
	var ly float64
	for x := range w {
		py := mid - amp*lerpRow(v.wave, float64(x)/float64(max(1, w-1)))
		if x > 0 {
			v.line(float64(x-1), ly, float64(x), py, col)
		}
		ly = py
	}
}

// drawOrb:
//
//	the sphere's surface   the spectrum, bass round the middle
//	the ring round it      bass
//	the glow behind        vocal
//	spikes                 the drum
//	the stars' speed       air
//
// In words: a sphere of glowing points that swells with the spectrum and
// turns slowly, in a starfield that rushes past on loud parts. Beats blow
// sparks off its surface and send a ring of light out; everything leaves a
// short trail.
func (v *visualizer) drawOrb(dt float64) {
	f := dt * 30
	zoom := 1 - (0.012+0.02*v.level+0.05*v.beat)*f*v.strength
	v.warp(math.Pow(0.62, f), func(u, w float64) (float64, float64) { return u * zoom, w * zoom })
	w := v.w
	bgc := v.pal[0].mul(0.012)
	par(v.h2, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := range w {
				fu, fv := v.unit(x, y)
				r := math.Sqrt(fu*fu + fv*fv)
				i := y*w + x
				v.fb[i] = v.fb[i].add(bgc).add(v.pal[1].mul(expNeg(r*2.2) * (0.02 + 0.1*v.voice[voiceVocal])))
			}
		}
	})
	// Stars fly at the camera: the scene's own moving backdrop, only when
	// that is the one asked for.
	speed := 0.3 + (3.5*v.voice[voiceAir]+3*v.pulse[voiceAir])*v.strength
	for i := range v.stars {
		if v.backdrop != "sparks" {
			break
		}
		st := &v.stars[i]
		st.z -= dt * speed
		if st.z < 0.05 {
			st.x, st.y, st.z = v.rnd()*2-1, v.rnd()*2-1, 4
		}
		px, py := v.pix(st.x/st.z*0.9, st.y/st.z*0.9)
		b := clamp01(1.2-st.z/4) * 0.6
		v.splat(px, py, vizWhite.mix(v.pal[2], 0.3).mul(b))
	}
	// The orb.
	gain := v.dense()
	ry, rx := v.t*0.35+v.twirl*2, 0.35*math.Sin(v.t*0.21)
	cy, sy := math.Cos(ry), math.Sin(ry)
	cx, sx := math.Cos(rx), math.Sin(rx)
	const cam, size = 3.0, 0.6
	turn := func(p vec3) vec3 {
		x, z := p.x*cy-p.z*sy, p.x*sy+p.z*cy
		y, z := p.y*cx-z*sx, p.y*sx+z*cx
		return vec3{x, y, z}
	}
	for i, p := range v.orb {
		// Neighbouring bands averaged: lone tones would make spikes.
		ay := math.Abs(p.y)
		band := (v.bandAt(ay-0.06) + v.bandAt(ay) + v.bandAt(ay+0.06)) / 3
		// The drum drives spikes out of the surface.
		spike := math.Sin(p.x*7+v.t*1.3) * math.Sin(p.y*9-v.t) * math.Sin(p.z*8)
		k := size * (0.85 + band*0.45 + v.beat*0.1 + (0.1*v.drive+0.5*v.kick)*spike*spike)
		q := turn(vec3{p.x * k, p.y * k, p.z * k})
		fp := cam / (cam + q.z)
		px, py := v.pix(q.x*fp, q.y*fp)
		depth := clamp01(0.75 - q.z*0.6)
		c := v.palAt(p.y*0.9 + band*0.6 + v.t*0.05).mul(depth * (0.1 + band*0.35) * gain)
		v.splat(px, py, c)
		// A beat blows some of the surface away.
		if v.onBeat && i%6 == v.beatN%6 {
			sp := 1 + v.rnd()*2.4
			n := turn(p)
			v.parts = append(v.parts, vizPart{
				x: q.x, y: q.y, z: q.z, vx: n.x * sp, vy: n.y * sp, vz: n.z * sp,
				life: 1, hue: p.y*0.9 + v.t*0.05,
			})
		}
	}
	v.stepParts(dt, 1.1, 1.4)
	for i := range v.parts {
		p := &v.parts[i]
		if p.z < -cam+0.3 {
			p.seen = false // behind the camera: no streak across the screen when it comes back
			continue
		}
		fp := cam / (cam + p.z)
		px, py := v.pix(p.x*fp, p.y*fp)
		v.drawPart(p, px, py, v.palAt(p.hue).mix(vizWhite, 0.3).mul(p.life*0.8*gain))
	}
	// The ring of light a beat sends out, and a thin one that stays.
	ring := func(rr, tilt float64, c vcol) {
		for a := 0.0; a < 2*math.Pi; a += 0.7 / (rr * v.sc) {
			px, py := v.pix(math.Cos(a)*rr, math.Sin(a)*rr*tilt)
			v.splat(px, py, c)
		}
	}
	ring(size*(1.25+0.5*v.voice[voiceBass]+0.15*v.pulse[voiceBass]), 0.32, v.pal[2].mul(0.15+0.5*v.voice[voiceBass]))
	if v.bigT < 0.9 {
		ring(size*(1.1+v.bigT*4), 1, v.pal[1].mix(vizWhite, 0.3).mul(0.5*v.bigS*sq(1-v.bigT/0.9)))
	}
	// The orb is big and in the middle of the screen: loud, it must not
	// turn into a lamp.
	v.softCap(0.5)
}

// softCap holds the picture's brightness under lim: what is under 0.6 of
// it stays as it is, the rest is pressed into what is left, keeping its hue.
func (v *visualizer) softCap(lim float64) {
	knee := lim * 0.6
	par(v.h2, func(y0, y1 int) {
		for i := y0 * v.w; i < y1*v.w; i++ {
			c := v.fb[i]
			l := math.Max(c.r, math.Max(c.g, c.b))
			if l <= knee {
				continue
			}
			nl := knee + (lim-knee)*(1-expNeg((l-knee)/(lim-knee)))
			v.fb[i] = c.mul(nl / l)
		}
	})
}

// drawRibbons: five ribbons, one for each voice, from the bass (the lowest
// on the screen at rest) to the air: each swells and swings with its own.
// The sparks behind them go with the air.
//
// In words: soft glowing ribbons that sway across the screen, each one
// swelling with its own part of the spectrum, over drifting sparks.
func (v *visualizer) drawRibbons() {
	w, h2 := v.w, v.h2
	const n = 5
	type ribbon struct {
		energy, speed, freq, phase, off float64
		reach                           float64 // how far its halo goes
		col                             vcol
	}
	var rs [n]ribbon
	for i := range n {
		fi := float64(i)
		rs[i] = ribbon{
			energy: v.voice[n-1-i],
			speed:  0.5 + fi*0.17,
			freq:   0.9 + fi*0.23,
			phase:  fi * 1.7,
			off:    (fi - 2) * 0.13,
			reach:  0.12 + v.voice[n-1-i]*0.1,
			col:    v.palAt(fi*0.65 + v.t*0.04),
		}
	}
	top, bot := v.pal[0].mul(0.03), v.pal[1].mul(0.12)
	// The backdrop's shade down the picture, once a frame.
	if len(v.ribbonBg) != h2 {
		v.ribbonBg = make([]vcol, h2)
	}
	for y := range h2 {
		v.ribbonBg[y] = top.mix(bot, float64(y)/float64(h2))
	}
	bg := v.ribbonBg
	par(w, func(x0, x1 int) {
		var xs [n * 2]float64 // per column: ribbon centre and thickness
		// Down a column the scene moves on by the same step each row, so a
		// halo, e^(-|d|/reach), only grows or shrinks by the same factor
		// from one row to the next: it is worked out once a column (and
		// again where the column crosses the ribbon), not every pixel.
		step := v.ya / v.sc
		var halo, grow, fade, kLit, kGlow, kCore, kThread [n]float64
		for x := x0; x < x1; x++ {
			u, fv0 := v.unit(x, 0)
			for i, r := range rs {
				depth := 0.6 + 0.4*math.Sin(u*0.7+v.t*0.3+r.phase)
				amp := 0.05 + r.energy*0.42
				y := r.off + amp*ribbonWave(u, v.t, r.freq, r.speed, r.phase)
				y += v.pulse[n-1-i] * 0.07 * math.Sin(u*9+r.phase*3+v.t*6) // a hit in its voice puts a shiver in it
				th := (0.008 + 0.06*depth*(0.15+1.2*r.energy)) * depth
				xs[i*2], xs[i*2+1] = y, th
				halo[i] = expNeg(math.Abs(fv0-y) / r.reach)
				grow[i], fade[i] = math.Exp(step/r.reach), math.Exp(-step/r.reach)
				// What stays the same down the column: how strong its
				// light is, from its thickness and its voice.
				kLit[i] = 0.5 + th*8
				kGlow[i] = 0.12 * (0.3 + r.energy) * kLit[i]
				kCore[i] = 0.25 + r.energy*1.3
				// The fine threads along the core, as strong as the
				// pixels can show them: on a thin ribbon at a coarse
				// size they change faster than a pixel a row and only
				// came out as a string of beads.
				kThread[i] = 0.25 * clamp01((2.6-9*step/th)/1.4)
			}
			for y := range h2 {
				fv := fv0 + float64(y)*step
				c := bg[y]
				for i := range rs {
					r := &rs[i]
					d := fv - xs[i*2]
					th := xs[i*2+1]
					light := halo[i] * kGlow[i]
					switch next := d + step; {
					case next < 0:
						halo[i] *= grow[i]
					case d >= 0:
						halo[i] *= fade[i]
					default:
						halo[i] = expNeg(next / r.reach)
					}
					// The core, with fine threads running along it, is
					// worked out only near the ribbon: four thicknesses
					// out it is under a ten-millionth.
					if q := d / th; q*q < 16 {
						light += expNeg(q*q) * (0.75 + kThread[i]*math.Sin(q*9+u*3+v.t)) * kCore[i] * kLit[i]
					}
					c.r += r.col.r * light
					c.g += r.col.g * light
					c.b += r.col.b * light
				}
				v.fb[y*w+x] = c
			}
		}
	})
	// Sparks drift up, brighter with the treble: the scene's own moving
	// backdrop, only when that is the one asked for.
	for i := range v.stars {
		if v.backdrop != "sparks" {
			break
		}
		st := &v.stars[i]
		st.y -= 0.004 * (1 + 5*v.voice[voiceAir]) * (0.5 + st.z/4)
		if st.y < -1 {
			st.y, st.x = 1, v.rnd()*2-1
		}
		px := (st.x*0.5 + 0.5) * float64(w)
		py := (st.y*0.5 + 0.5) * float64(h2)
		tw := 0.5 + 0.5*math.Sin(v.t*4+st.tw)
		v.splat(px, py, v.palAt(st.tw).mix(vizWhite, 0.4).mul(tw*(0.15+1.5*v.voice[voiceAir]+2*v.pulse[voiceAir])))
	}
}

// drawBackdrop lays the chosen backdrop (see visualizer.backdrop) under the
// finished picture: tiny squares, as behind the ribbons, that show only
// where the scene is dark. "stars" stand still and only twinkle a little,
// on the wall clock rather than the music's: whatever the scene does, the
// eye has something steady to hold on to. "sparks" drift up slowly.
func (v *visualizer) drawBackdrop() {
	k := v.backdropK
	if k < 1 && v.backdropWas != v.backdrop {
		v.drawBackdropOf(v.backdropWas, 1-k) // the last one going
	}
	v.drawBackdropOf(v.backdrop, k)
}

// drawBackdropOf draws the backdrop of that kind, as strong as level (0..1).
func (v *visualizer) drawBackdropOf(kind string, level float64) {
	if kind != "stars" && kind != "sparks" || v.w <= 0 || v.h2 <= 0 || level <= 0 {
		return
	}
	if kind == "sparks" && (v.scene == sceneRibbons || v.scene == sceneOrb) {
		return // these have moving sparks of their own (drawn only then)
	}
	k := max(1, v.px)
	n := min(700, max(40, (v.w/k)*(v.h2/k)/90))
	for len(v.dots) < n {
		v.dots = append(v.dots, vizStar{v.rnd(), v.rnd(), v.rnd(), v.rnd() * 6.28})
	}
	w, h2 := float64(v.w), float64(v.h2)
	px := float64(k)
	for i := range v.dots[:n] {
		d := &v.dots[i]
		x0, y0 := d.x*w, d.y*h2
		var b float64
		if kind == "sparks" {
			d.y -= 0.0025 * (0.4 + d.z) * (1 + 2*v.voice[voiceAir]*v.strength)
			if d.y < 0 {
				d.y, d.x = 1, v.rnd()
				x0, y0 = d.x*w, d.y*h2
			}
			b = (0.2 + 0.3*d.z) * (0.6 + 0.4*math.Sin(v.clock*2+d.tw))
		} else {
			b = (0.16 + 0.3*d.z) * (0.75 + 0.25*math.Sin(v.clock*0.9+d.tw))
		}
		c := v.pal[i%3].mix(vizWhite, 0.55).mul(b * level)
		// A point of light, round and soft, where it is to a fraction of
		// a pixel, with a faint glow round it; the nearer ones larger.
		// (A cell pixel's square, put where the picture was already
		// finished, sat on it like a sticker, and the sparks went up
		// a square at a time.) A spark draws the way it went this frame.
		r := px * (0.5 + 0.35*d.z)
		x1, y1 := d.x*w, d.y*h2
		v.backdropDot(x0, y0, x1, y1, r, c.mul(1.7))
		v.backdropDot(x0, y0, x1, y1, r*3, c.mul(0.18))
	}
}

// backdropDot adds a soft round point of light of radius r (pixels) and
// colour c (its middle's) to the finished picture, drawn from (x0, y0) to
// (x1, y1) when it moved: behind the picture, as much as it is dark there.
func (v *visualizer) backdropDot(x0, y0, x1, y1, r float64, c vcol) {
	steps := max(1, int(math.Ceil(math.Hypot(x1-x0, y1-y0)/math.Max(0.5, r*0.5))))
	c = c.mul(1 / float64(steps))
	for s := range steps {
		f := 0.0
		if steps > 1 {
			f = float64(s) / float64(steps-1)
		}
		cx, cy := x0+(x1-x0)*f, y0+(y1-y0)*f
		xa, xb := max(0, int(math.Floor(cx-r))), min(v.w-1, int(math.Ceil(cx+r)))
		ya, yb := max(0, int(math.Floor(cy-r))), min(v.h2-1, int(math.Ceil(cy+r)))
		for y := ya; y <= yb; y++ {
			dy := (float64(y) + 0.5 - cy) / r
			for x := xa; x <= xb; x++ {
				dx := (float64(x) + 0.5 - cx) / r
				q := 1 - (dx*dx + dy*dy)
				if q <= 0 {
					continue
				}
				o := &v.out[y*v.w+x]
				lum := math.Max(o.r, math.Max(o.g, o.b))
				*o = o.add(c.mul(q * q * (1 - smooth(0.05, 0.45, lum))))
			}
		}
	}
}

// ribbonWave is the line a ribbon follows: where it is, -1..1 of its
// swing, at u along it and time t. The visualizer's ribbons and the ones
// behind the lyrics (see lines.go) are the same curve.
func ribbonWave(u, t, freq, speed, phase float64) float64 {
	return 0.6*math.Sin(u*freq+t*speed+phase) + 0.4*math.Sin(u*2.3-t*0.8+phase*2)
}
