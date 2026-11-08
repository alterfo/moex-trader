package main

func findEntry(f *future, opts []*option, expiry string, dte int, slack int, minVol float64, useSettle bool) (string, *option, *option, bool) {
	ei, ok := f.at[expiry]
	if !ok {
		return "", nil, nil, false
	}
	for extra := 0; extra <= slack; extra++ {
		idx := ei - dte + extra
		if idx < 0 {
			return "", nil, nil, false
		}
		day := f.dates[idx]
		fs := f.settle[day]
		if fs <= 0 {
			continue
		}
		call, put := atmPair(opts, day, fs)
		if call == nil || put == nil {
			continue
		}
		if !useSettle {
			if _, ok := call.wap[day]; !ok {
				continue
			}
			if _, ok := put.wap[day]; !ok {
				continue
			}
			if minVol > 0 && (call.vol[day] < minVol || put.vol[day] < minVol) {
				continue
			}
		}
		return day, call, put, true
	}
	return "", nil, nil, false
}

func premiumOf(o *option, day string, useSettle bool) (float64, bool) {
	if useSettle {
		v, ok := o.settle[day]
		return v, ok
	}
	v, ok := o.wap[day]
	return v, ok
}

func premiumAt(call, put *option, day string, useSettle bool) (float64, bool) {
	cv, ok1 := premiumOf(call, day, useSettle)
	pv, ok2 := premiumOf(put, day, useSettle)
	if !ok1 || !ok2 {
		return 0, false
	}
	return cv + pv, true
}
