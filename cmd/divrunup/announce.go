package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	boardRec  = regexp.MustCompile(`(?i)(совет директоров|наблюдательный совет|набсовет|(^|[^\p{L}])сд([^\p{L}]|$))[^.]{0,60}рекоменд`)
	moscow    = time.FixedZone("MSK", 3*3600)
	annCutHrs = 18*60 + 30
)

type headline struct {
	day string
	ts  int64
}

func loadAliases(path string) map[string][]string {
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}
	var m map[string][]string
	if err := json.Unmarshal(raw, &m); err != nil {
		log.Fatal(err)
	}
	out := map[string][]string{}
	for t, names := range m {
		for _, n := range names {
			r := []rune(strings.ToLower(strings.TrimSpace(n)))
			if len(r) > 5 {
				r = r[:5]
			}
			if len(r) >= 3 {
				out[t] = append(out[t], string(r))
			}
		}
	}
	return out
}

func loadHeadlines(path string, stems map[string][]string) map[string][]headline {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	out := map[string][]headline{}
	for sc.Scan() {
		var r struct {
			Ticker string `json:"ticker"`
			Title  string `json:"title"`
			TS     int64  `json:"published_ts"`
		}
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		low := strings.ToLower(r.Title)
		if !strings.Contains(low, "дивиденд") || !boardRec.MatchString(r.Title) {
			continue
		}
		hit := false
		for _, s := range stems[r.Ticker] {
			if strings.Contains(low, s) {
				hit = true
				break
			}
		}
		if !hit {
			continue
		}
		t := time.Unix(r.TS, 0).In(moscow)
		d := t
		if t.Hour()*60+t.Minute() > annCutHrs {
			d = t.AddDate(0, 0, 1)
		}
		out[r.Ticker] = append(out[r.Ticker], headline{day: d.Format("2006-01-02"), ts: r.TS})
	}
	for t := range out {
		sort.Slice(out[t], func(i, j int) bool { return out[t][i].ts < out[t][j].ts })
	}
	return out
}

func announce(events []event, dates []string, lasts map[string][]string, newsPath, aliasPath string) {
	hl := loadHeadlines(newsPath, loadAliases(aliasPath))
	var ver, unver []event
	var leads []int
	matched := 0
	for i := range events {
		e := &events[i]
		e.ann = -1
		prev := ""
		for _, l := range lasts[e.ticker] {
			if l < e.last && l > prev {
				prev = l
			}
		}
		lo, _ := time.Parse("2006-01-02", e.last)
		start := lo.AddDate(0, 0, -150).Format("2006-01-02")
		if prev != "" && prev > start {
			start = prev
		}
		for _, h := range hl[e.ticker] {
			if h.day > start && h.day < e.last {
				k := sort.SearchStrings(dates, h.day)
				if k < len(dates) {
					e.ann = k
				}
				break
			}
		}
		if e.ann >= 0 {
			matched++
		}
		entry := e.li - 11
		if e.ann >= 0 && e.ann <= entry {
			ver = append(ver, *e)
			leads = append(leads, entry-e.ann)
		} else {
			unver = append(unver, *e)
		}
	}
	sort.Ints(leads)
	fmt.Printf("DA1 announcement dates: %d events, matched %d, verified (announced at or before the entry close) %d, unverified %d\n", len(events), matched, len(ver), len(unver))
	if len(leads) > 0 {
		fmt.Printf("DA1 lead in trading days from the announcement to the entry: min %d, quartiles %d / %d / %d, max %d\n", leads[0], leads[len(leads)/4], leads[len(leads)/2], leads[3*len(leads)/4], leads[len(leads)-1])
	}
	fmt.Println("DA2 economics, verified events only")
	economics(ver)
	fmt.Println("DA3 economics, unverified events (descriptive)")
	economics(unver)
}
