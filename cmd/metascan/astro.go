package main

import (
	"math"
	"time"
)

type elems struct {
	a, e, i, l, w, o       float64
	da, de, di, dl, dw, do float64
}

var earthEl = elems{1.00000261, 0.01671123, -0.00001531, 100.46457166, 102.93768193, 0.0, 0.00000562, -0.00004392, -0.01294668, 35999.37244981, 0.32327364, 0.0}
var saturnEl = elems{9.53667594, 0.05386179, 2.48599187, 49.95424423, 92.59887831, 113.66242448, -0.00125060, -0.00050991, 0.00193609, 1222.49362201, -0.41897216, -0.28867794}

func rad(x float64) float64 { return x * math.Pi / 180 }

func julian(t time.Time) float64 {
	return float64(t.Unix())/86400 + 2440587.5
}

func helio(el elems, T float64) (float64, float64, float64) {
	a := el.a + el.da*T
	e := el.e + el.de*T
	inc := rad(el.i + el.di*T)
	L := el.l + el.dl*T
	w := el.w + el.dw*T
	o := el.o + el.do*T
	M := math.Mod(L-w, 360)
	if M > 180 {
		M -= 360
	}
	if M < -180 {
		M += 360
	}
	Mr := rad(M)
	E := Mr + e*math.Sin(Mr)
	for k := 0; k < 30; k++ {
		E -= (E - e*math.Sin(E) - Mr) / (1 - e*math.Cos(E))
	}
	xp := a * (math.Cos(E) - e)
	yp := a * math.Sqrt(1-e*e) * math.Sin(E)
	argp := rad(w - o)
	om := rad(o)
	x := (math.Cos(argp)*math.Cos(om)-math.Sin(argp)*math.Sin(om)*math.Cos(inc))*xp + (-math.Sin(argp)*math.Cos(om)-math.Cos(argp)*math.Sin(om)*math.Cos(inc))*yp
	y := (math.Cos(argp)*math.Sin(om)+math.Sin(argp)*math.Cos(om)*math.Cos(inc))*xp + (-math.Sin(argp)*math.Sin(om)+math.Cos(argp)*math.Cos(om)*math.Cos(inc))*yp
	z := (math.Sin(argp)*math.Sin(inc))*xp + (math.Cos(argp)*math.Sin(inc))*yp
	return x, y, z
}

func saturnLongitude(t time.Time) float64 {
	T := (julian(t) - 2451545.0) / 36525
	ex, ey, _ := helio(earthEl, T)
	sx, sy, _ := helio(saturnEl, T)
	lon := math.Atan2(sy-ey, sx-ex) * 180 / math.Pi
	if lon < 0 {
		lon += 360
	}
	return lon
}

func moonAge(t time.Time) float64 {
	ref := julian(time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC))
	age := math.Mod(julian(t)-ref, 29.530588853)
	if age < 0 {
		age += 29.530588853
	}
	return age
}

var mercuryEl = elems{0.38709843, 0.20563661, 7.00559432, 252.25166724, 77.45771895, 48.33961819, 0.0, 0.00002123, -0.00590158, 149472.67486623, 0.15940013, -0.12214182}

func mercuryLongitude(t time.Time) float64 {
	T := (julian(t) - 2451545.0) / 36525
	ex, ey, _ := helio(earthEl, T)
	mx, my, _ := helio(mercuryEl, T)
	lon := math.Atan2(my-ey, mx-ex) * 180 / math.Pi
	if lon < 0 {
		lon += 360
	}
	return lon
}

func mercuryRetrograde(t time.Time) bool {
	a := mercuryLongitude(t.Add(-12 * time.Hour))
	b := mercuryLongitude(t.Add(12 * time.Hour))
	d := b - a
	if d > 180 {
		d -= 360
	}
	if d < -180 {
		d += 360
	}
	return d < 0
}
