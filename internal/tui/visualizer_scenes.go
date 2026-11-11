package tui

import "math"

// The visualizer's scenes. Each one fills v.fb; see visualizer.go for what
// happens to it next.
//
// In every scene each element listens to a voice of its own (see voiceBass
// and the others): the comment of the scene says which.

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
	cu, cv := 0.18*sway*math.Sin(v.t*0.53), 0.12*sway*math.Cos(v.t*0.41)
	rot := v.t*0.12 + v.twirl
	core := v.pal[2].mix(vizWhite, 0.5)
	waveZ, waveA := 0.3+v.bigT*11, v.bigS*math.Exp(-v.bigT*2.2)
	// The tube is round only at rest: the drum squeezes it into lobes.
	const lobes = 3.0
	squeeze := 0.03*v.drive + 0.22*v.kick
	par(v.h2, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := range w {
				px, py := v.unit(x, y)
				px, py = px-cu, py-cv
				r := math.Sqrt(px*px+py*py) + 1e-4
				a := math.Atan2(py, px)/(2*math.Pi) + 0.5
				r *= 1 + squeeze*math.Sin((a+rot)*2*math.Pi*lobes+v.t*1.5)
				z := 0.55 / r
				sa := a + rot + z*0.05
				d := z*3 + v.t*2.4
				mirror := math.Abs(math.Mod(sa*2+4, 2) - 1) // 0..1 around the tube, twice
				band := v.bandAt(mirror)
				ring := math.Exp(-sq(math.Mod(d, 1)-0.5) * (90 - 55*v.voice[voiceBass]))
				strip := math.Mod(sa*24+48, 1)
				edge := math.Exp(-sq(strip-0.5) * 70)
				panel := math.Mod(d*0.5, 1)
				lit := band * band * smooth(0.25, 0.9, panel+band*0.4) * (1 - smooth(0.8, 1, panel))
				bright := ring*(0.2+1.5*v.voice[voiceBass]+0.5*v.pulse[voiceBass]) + edge*(0.05+0.5*v.voice[voiceBody]) + lit*0.7
				// A spark on some strips: a bright head with a tail behind it.
				cell := sa*72 + 720
				if hsh := hash1(math.Mod(math.Floor(cell), 72)); hsh > 0.72 {
					f := math.Mod(d*0.3+hsh*31, 1)
					across := sq(math.Sin((cell - math.Floor(cell)) * math.Pi))
					bright += math.Exp(-f*9) * sq(across) * (0.1 + 1.6*v.voice[voiceAir] + 2*v.pulse[voiceAir])
				}
				fog := sq(clamp01(r * 1.6))
				c := v.palAt(d*0.05 + mirror*0.9).mul(bright * fog * (0.95 + v.beat*0.15))
				c = c.add(core.mul(math.Exp(-sq(z-waveZ)*1.1) * waveA * 0.45 * fog))
				c = c.add(core.mul(math.Exp(-r*r*(26-14*v.voice[voiceVocal])) * (0.1 + 0.7*v.voice[voiceVocal])))
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
					band := math.Mod((fy-sunY)/fh*14+v.t*0.4, 1)
					if r < 1 && (dy <= 0 || band >= 0.18+dy/sunR*0.3) {
						c = v.pal[1].mix(v.pal[2], clamp01(dy/sunR*0.5+0.5)).mul(1.15)
					} else {
						c = c.add(v.pal[1].mul((0.15 + 0.7*v.voice[voiceVocal]) * math.Exp(-math.Max(0, r-1)*(5-2.5*v.voice[voiceVocal]))))
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
					hgt *= 0.55 + 0.7*v.drive + 1.1*v.kick*math.Exp(-z*0.25) // the ground near you jumps with the drum
				}
				// The canyon's walls, beyond the spectrum.
				zs := z + scroll
				hgt += smooth(0.8, 1.6, bx) * (0.55 + 0.7*v.voice[voiceBody]) * (0.95 + 0.4*math.Sin(zs*0.8+lx*3) + 0.25*math.Sin(zs*2.3))
				sy := horizon + (camH-hgt)*proj/z
				if sy >= yb {
					continue
				}
				fog := math.Exp(-z * 0.085)
				gx := math.Abs(math.Mod(wx*1.3+100, 1) - 0.5)
				gz := math.Abs(math.Mod(zs*0.7+100, 1) - 0.5)
				grid := math.Max(math.Exp(-sq(0.5-gx)*400/(z*0.3+1)), math.Exp(-sq(0.5-gz)*300/(z*0.3+1)))
				grid *= math.Exp(-z * 0.09)                              // far lines would blur into a haze
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
	for _, p := range v.parts {
		px, py := v.pix(p.x, p.y)
		v.splat(px, py, v.palAt(p.hue).mix(vizWhite, 0.35).mul(p.life*0.9*gain))
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
func (v *visualizer) stepParts(dt, lifetime, drag float64) {
	k := math.Exp(-dt * drag)
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
				c := v.pal[0].mul(0.015).add(glow.mul(math.Exp(-d*5) * (1 - fx*0.6)))
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
				v.fb[i] = v.fb[i].add(bgc).add(v.pal[1].mul(math.Exp(-r*2.2) * (0.02 + 0.1*v.voice[voiceVocal])))
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
	for _, p := range v.parts {
		if p.z < -cam+0.3 {
			continue
		}
		fp := cam / (cam + p.z)
		px, py := v.pix(p.x*fp, p.y*fp)
		v.splat(px, py, v.palAt(p.hue).mix(vizWhite, 0.3).mul(p.life*0.8*gain))
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
			nl := knee + (lim-knee)*(1-math.Exp(-(l-knee)/(lim-knee)))
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
			col:    v.palAt(fi*0.65 + v.t*0.04),
		}
	}
	top, bot := v.pal[0].mul(0.03), v.pal[1].mul(0.12)
	par(w, func(x0, x1 int) {
		var xs [n * 2]float64 // per column: ribbon centre and thickness
		for x := x0; x < x1; x++ {
			u, _ := v.unit(x, 0)
			for i, r := range rs {
				depth := 0.6 + 0.4*math.Sin(u*0.7+v.t*0.3+r.phase)
				amp := 0.05 + r.energy*0.42
				y := r.off + amp*(0.6*math.Sin(u*r.freq+v.t*r.speed+r.phase)+0.4*math.Sin(u*2.3-v.t*0.8+r.phase*2))
				y += v.pulse[n-1-i] * 0.07 * math.Sin(u*9+r.phase*3+v.t*6) // a hit in its voice puts a shiver in it
				xs[i*2], xs[i*2+1] = y, (0.008+0.06*depth*(0.15+1.2*r.energy))*depth
			}
			for y := range h2 {
				_, fv := v.unit(x, y)
				c := top.mix(bot, float64(y)/float64(h2))
				for i, r := range rs {
					d := fv - xs[i*2]
					th := xs[i*2+1]
					core := math.Exp(-sq(d / th))
					halo := math.Exp(-math.Abs(d)/(0.12+r.energy*0.1)) * 0.12
					// Fine threads run along each ribbon.
					thread := 0.75 + 0.25*math.Sin(d/th*9+u*3+v.t)
					c = c.add(r.col.mul((core*thread*(0.25+r.energy*1.3) + halo*(0.3+r.energy)) * (0.5 + th*8)))
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
	if v.backdrop != "stars" && v.backdrop != "sparks" || v.w <= 0 || v.h2 <= 0 {
		return
	}
	if v.backdrop == "sparks" && (v.scene == sceneRibbons || v.scene == sceneOrb) {
		return // these have moving sparks of their own (drawn only then)
	}
	n := min(700, max(40, v.w*v.h2/90))
	for len(v.dots) < n {
		v.dots = append(v.dots, vizStar{v.rnd(), v.rnd(), v.rnd(), v.rnd() * 6.28})
	}
	w, h2 := v.w, v.h2
	for i := range v.dots[:n] {
		d := &v.dots[i]
		var b float64
		if v.backdrop == "sparks" {
			d.y -= 0.0025 * (0.4 + d.z) * (1 + 2*v.voice[voiceAir]*v.strength)
			if d.y < 0 {
				d.y, d.x = 1, v.rnd()
			}
			b = (0.2 + 0.3*d.z) * (0.6 + 0.4*math.Sin(v.clock*2+d.tw))
		} else {
			b = (0.16 + 0.3*d.z) * (0.75 + 0.25*math.Sin(v.clock*0.9+d.tw))
		}
		x, y := int(d.x*float64(w)), int(d.y*float64(h2))
		if x < 0 || x >= w || y < 0 || y >= h2 {
			continue
		}
		c := v.pal[i%3].mix(vizWhite, 0.55).mul(b)
		o := &v.out[y*w+x]
		// Behind the picture: as much as it is dark there.
		lum := math.Max(o.r, math.Max(o.g, o.b))
		*o = o.add(c.mul(1 - smooth(0.05, 0.45, lum)))
	}
}
