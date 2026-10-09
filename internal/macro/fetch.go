package macro

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultISSBase = "https://iss.moex.com/iss"
	DefaultCBRBase = "https://www.cbr.ru"
	issPageSize    = 100
	maxPages       = 200
)

var brentMonthCodes = []byte("FGHJKMNQUVXZ")

type Fetcher struct {
	ISSBase string
	CBRBase string
	HTTP    *http.Client
}

func NewFetcher() *Fetcher {
	return &Fetcher{ISSBase: DefaultISSBase, CBRBase: DefaultCBRBase, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

func (f *Fetcher) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; moex-trader)")
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("macro: GET %s: status %d", rawURL, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

type issHistory struct {
	History struct {
		Columns []string            `json:"columns"`
		Data    [][]json.RawMessage `json:"data"`
	} `json:"history"`
}

func (f *Fetcher) issHistory(ctx context.Context, path string, from, till time.Time) (Series, error) {
	var out Series
	for page := 0; page < maxPages; page++ {
		q := url.Values{}
		q.Set("iss.meta", "off")
		q.Set("history.columns", "TRADEDATE,CLOSE")
		q.Set("start", strconv.Itoa(page*issPageSize))
		if !from.IsZero() {
			q.Set("from", from.Format("2006-01-02"))
		}
		if !till.IsZero() {
			q.Set("till", till.Format("2006-01-02"))
		}
		body, err := f.get(ctx, f.ISSBase+path+".json?"+q.Encode())
		if err != nil {
			return nil, err
		}
		var parsed issHistory
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("macro: decode %s: %w", path, err)
		}
		if len(parsed.History.Data) == 0 {
			break
		}
		for _, row := range parsed.History.Data {
			if len(row) < 2 {
				continue
			}
			var date string
			var closeValue *float64
			if json.Unmarshal(row[0], &date) != nil || json.Unmarshal(row[1], &closeValue) != nil || closeValue == nil || *closeValue <= 0 {
				continue
			}
			day, err := time.Parse("2006-01-02", date)
			if err != nil {
				continue
			}
			out = append(out, Point{Date: day, Value: *closeValue})
		}
	}
	return out.Sorted(), nil
}

func (f *Fetcher) IMOEX(ctx context.Context, from, till time.Time) (Series, error) {
	return f.issHistory(ctx, "/history/engines/stock/markets/index/securities/IMOEX", from, till)
}

func (f *Fetcher) CNYRUB(ctx context.Context, from, till time.Time) (Series, error) {
	return f.issHistory(ctx, "/history/engines/currency/markets/selt/boards/CETS/securities/CNYRUB_TOM", from, till)
}

func brentContracts(from, till time.Time) []string {
	var out []string
	for year := from.Year() - 1; year <= till.Year()+1; year++ {
		for _, code := range brentMonthCodes {
			out = append(out, fmt.Sprintf("BR%c%d", code, year%10))
		}
	}
	return out
}

func (f *Fetcher) Brent(ctx context.Context, from, till time.Time) (Series, error) {
	contracts := make(map[string]Series)
	for _, name := range brentContracts(from, till) {
		series, err := f.issHistory(ctx, "/history/engines/futures/markets/forts/securities/"+name, from.AddDate(0, 0, -45), till)
		if err != nil {
			return nil, fmt.Errorf("macro: brent contract %s: %w", name, err)
		}
		if len(series) > 0 {
			contracts[name] = series
		}
	}
	return ChainFront(contracts, 7*24*time.Hour), nil
}

func ChainFront(contracts map[string]Series, minLife time.Duration) Series {
	closes := make(map[string]map[time.Time]float64, len(contracts))
	lastDay := make(map[string]time.Time, len(contracts))
	daySet := make(map[time.Time]struct{})
	for name, series := range contracts {
		m := make(map[time.Time]float64, len(series))
		for _, p := range series {
			m[p.Date] = p.Value
			daySet[p.Date] = struct{}{}
		}
		closes[name] = m
		lastDay[name] = series[len(series)-1].Date
	}
	days := make([]time.Time, 0, len(daySet))
	for d := range daySet {
		days = append(days, d)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })

	names := make([]string, 0, len(contracts))
	for name := range contracts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if lastDay[names[i]].Equal(lastDay[names[j]]) {
			return names[i] < names[j]
		}
		return lastDay[names[i]].Before(lastDay[names[j]])
	})

	var out Series
	level := 100.0
	var prev time.Time
	for _, day := range days {
		if prev.IsZero() {
			out = append(out, Point{Date: day, Value: level})
			prev = day
			continue
		}
		chosen := ""
		for _, name := range names {
			_, hasToday := closes[name][day]
			_, hasPrev := closes[name][prev]
			if !hasToday || !hasPrev {
				continue
			}
			if lastDay[name].Sub(day) >= minLife {
				chosen = name
				break
			}
			if chosen == "" {
				chosen = name
			}
		}
		if chosen == "" {
			continue
		}
		level *= closes[chosen][day] / closes[chosen][prev]
		out = append(out, Point{Date: day, Value: level})
		prev = day
	}
	return out
}

var keyRateRow = regexp.MustCompile(`<td>\s*(\d{2}\.\d{2}\.\d{4})\s*</td>\s*<td>\s*([\d\s]+[,.]?\d*)\s*</td>`)

func (f *Fetcher) KeyRate(ctx context.Context, from, till time.Time) (Series, error) {
	q := url.Values{}
	q.Set("UniDbQuery.Posted", "True")
	q.Set("UniDbQuery.From", from.Format("02.01.2006"))
	q.Set("UniDbQuery.To", till.Format("02.01.2006"))
	body, err := f.get(ctx, f.CBRBase+"/hd_base/KeyRate/?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var out Series
	for _, m := range keyRateRow.FindAllStringSubmatch(string(body), -1) {
		day, err := time.Parse("02.01.2006", m[1])
		if err != nil {
			continue
		}
		value, err := strconv.ParseFloat(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(m[2]), " ", ""), ",", "."), 64)
		if err != nil {
			continue
		}
		out = append(out, Point{Date: day, Value: value})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("macro: no key rate rows parsed from cbr.ru")
	}
	return out.Sorted(), nil
}

func (f *Fetcher) All(ctx context.Context, from, till time.Time) (*Provider, error) {
	imoex, err := f.IMOEX(ctx, from, till)
	if err != nil {
		return nil, fmt.Errorf("macro: imoex: %w", err)
	}
	cnyrub, err := f.CNYRUB(ctx, from, till)
	if err != nil {
		return nil, fmt.Errorf("macro: cnyrub: %w", err)
	}
	brent, err := f.Brent(ctx, from, till)
	if err != nil {
		return nil, fmt.Errorf("macro: brent: %w", err)
	}
	keyRate, err := f.KeyRate(ctx, from.AddDate(0, 0, -120), till)
	if err != nil {
		return nil, fmt.Errorf("macro: key rate: %w", err)
	}
	return NewProvider(imoex, cnyrub, brent, keyRate), nil
}

func (p *Provider) Refresh(ctx context.Context, f *Fetcher, now time.Time) error {
	imoex, cnyrub, brent, keyRate := p.Snapshot()
	tailFrom := func(s Series) time.Time {
		if last, ok := s.Last(); ok {
			return last.Date.AddDate(0, 0, -10)
		}
		return now.AddDate(-2, 0, 0)
	}
	merge := func(old, fresh Series) Series { return append(append(Series(nil), old...), fresh...).Sorted() }

	newIMOEX, err := f.IMOEX(ctx, tailFrom(imoex), now)
	if err != nil {
		return fmt.Errorf("macro: refresh imoex: %w", err)
	}
	newUSD, err := f.CNYRUB(ctx, tailFrom(cnyrub), now)
	if err != nil {
		return fmt.Errorf("macro: refresh cnyrub: %w", err)
	}
	newKey, err := f.KeyRate(ctx, tailFrom(keyRate), now)
	if err != nil {
		return fmt.Errorf("macro: refresh key rate: %w", err)
	}
	newBrent, err := f.Brent(ctx, tailFrom(brent).AddDate(0, 0, -60), now)
	if err != nil {
		return fmt.Errorf("macro: refresh brent: %w", err)
	}
	mergedBrent := brent
	if len(newBrent) > 1 {
		mergedBrent = rebaseBrent(brent, newBrent)
	}
	p.Replace(merge(imoex, newIMOEX), merge(cnyrub, newUSD), mergedBrent, merge(keyRate, newKey))
	return nil
}

func rebaseBrent(old, fresh Series) Series {
	var anchorOld, anchorFresh float64
	var anchorDate time.Time
	for _, p := range old {
		if v, ok := fresh.valueOnOrBefore(p.Date); ok && !fresh[0].Date.After(p.Date) {
			anchorOld, anchorFresh, anchorDate = p.Value, v, p.Date
		}
	}
	if anchorDate.IsZero() || anchorFresh <= 0 {
		return old
	}
	scale := anchorOld / anchorFresh
	out := make(Series, 0, len(old)+len(fresh))
	for _, p := range old {
		if !p.Date.After(anchorDate) {
			out = append(out, p)
		}
	}
	for _, p := range fresh {
		if p.Date.After(anchorDate) {
			out = append(out, Point{Date: p.Date, Value: p.Value * scale})
		}
	}
	return out.Sorted()
}
