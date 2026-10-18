package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	parquet "github.com/parquet-go/parquet-go"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

const (
	noTitle          = "no title"
	maxBodyTitleRune = 280
	minTagAliasCount = 2
	titleKeyRunes    = 160
)

var (
	tagQuoted   = regexp.MustCompile(`'([^']*)'|"([^"]*)"`)
	tagNameTick = regexp.MustCompile(`^(.*?)\s+([A-Z][A-Z0-9]{1,5})$`)
	tagParens   = regexp.MustCompile(`\(([^)]*)\)`)
	tagFormerRe = regexp.MustCompile(`(?i)^(ранее|бывш\.?|бывшая|бывший)\s+`)
	nonWord     = regexp.MustCompile(`[^\p{L}\p{N}]+`)
)

type aliasStat struct {
	matches int
	correct int
}

type universeRun struct {
	uni       *news.Universe
	tickerSet map[string]bool
	out       *os.File
	seen      map[string]bool
	opts      options
	written   int
	perTicker map[string]int
	stats     map[string]*aliasStat
	tagged    int
	recalled  int
	skipped   int
}

func normTitle(title string) string {
	n := strings.Join(strings.Fields(strings.ToLower(nonWord.ReplaceAllString(title, " "))), " ")
	rs := []rune(n)
	if len(rs) > titleKeyRunes {
		rs = rs[:titleKeyRunes]
	}
	return string(rs)
}

func titleKey(ticker, title string) string {
	return ticker + "|t:" + normTitle(title)
}

func hashID(title, body string) string {
	sum := sha256.Sum256([]byte(title + "|" + body))
	return hex.EncodeToString(sum[:])[:16]
}

func parseTags(raw string) []string {
	var tags []string
	for _, m := range tagQuoted.FindAllStringSubmatch(raw, -1) {
		if m[1] != "" {
			tags = append(tags, m[1])
		} else if m[2] != "" {
			tags = append(tags, m[2])
		}
	}
	return tags
}

var renamedTickers = map[string]string{"YNDX": "YDEX", "TCSG": "T", "HHRU": "HEAD"}

func canonicalTag(ticker string) string {
	if renamed, ok := renamedTickers[ticker]; ok {
		return renamed
	}
	return ticker
}

func tagTickers(tags []string, set map[string]bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, tag := range tags {
		m := tagNameTick.FindStringSubmatch(strings.TrimSpace(tag))
		if m == nil {
			continue
		}
		ticker := canonicalTag(m[2])
		if !set[ticker] || seen[ticker] {
			continue
		}
		seen[ticker] = true
		out = append(out, ticker)
	}
	return out
}

func learnTagAliases(paths []string, set map[string]bool) (map[string][]string, error) {
	counts := map[string]map[string]int{}
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		pr := parquet.NewGenericReader[kasymRow](f)
		rows := make([]kasymRow, 1000)
		for {
			n, err := pr.Read(rows)
			for _, row := range rows[:n] {
				for _, tag := range parseTags(row.Tags) {
					m := tagNameTick.FindStringSubmatch(strings.TrimSpace(tag))
					if m == nil {
						continue
					}
					ticker := canonicalTag(m[2])
					if !set[ticker] {
						continue
					}
					if counts[ticker] == nil {
						counts[ticker] = map[string]int{}
					}
					for _, alias := range tagAliasVariants(m[1]) {
						counts[ticker][alias]++
					}
				}
			}
			if err != nil {
				if err == io.EOF {
					break
				}
				pr.Close()
				f.Close()
				return nil, err
			}
			if n == 0 {
				break
			}
		}
		pr.Close()
		f.Close()
	}
	out := map[string][]string{}
	for ticker, byAlias := range counts {
		for alias, c := range byAlias {
			if c >= minTagAliasCount {
				out[ticker] = append(out[ticker], alias)
			}
		}
		sort.Strings(out[ticker])
	}
	return out, nil
}

func tagAliasVariants(name string) []string {
	name = strings.TrimSpace(name)
	var variants []string
	for _, m := range tagParens.FindAllStringSubmatch(name, -1) {
		inner := strings.TrimSpace(tagFormerRe.ReplaceAllString(strings.TrimSpace(m[1]), ""))
		if inner != "" {
			variants = append(variants, inner)
		}
	}
	outer := strings.TrimSpace(tagParens.ReplaceAllString(name, " "))
	if outer != "" {
		variants = append(variants, outer)
	}
	return variants
}

func loadBlocklist(path string) (map[string]bool, error) {
	blocked := map[string]bool{}
	if path == "" {
		return blocked, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("parse blocklist %q: %w", path, err)
	}
	for _, a := range list {
		blocked[strings.Join(strings.Fields(strings.ToLower(a)), " ")] = true
	}
	return blocked, nil
}

func newUniverseRun(opts options, out *os.File, seen map[string]bool) (*universeRun, error) {
	names, err := news.LoadTickerNames(opts.namesPath)
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, n := range names {
		set[n.Ticker] = true
	}
	extra := map[string][]string{}
	for t, aliases := range news.DefaultAliases() {
		if set[t] {
			extra[t] = append([]string(nil), aliases...)
		}
	}
	learned, err := learnTagAliases(opts.parquets, set)
	if err != nil {
		return nil, err
	}
	for t, aliases := range learned {
		extra[t] = append(extra[t], aliases...)
	}
	if opts.aliasesOut != "" {
		raw, err := json.MarshalIndent(learned, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(opts.aliasesOut, raw, 0o644); err != nil {
			return nil, err
		}
	}
	blocked, err := loadBlocklist(opts.blocklist)
	if err != nil {
		return nil, err
	}
	uni := news.NewUniverse(news.UniverseConfig{Names: names, Extra: extra, Blocked: blocked})
	return &universeRun{
		uni: uni, tickerSet: set, out: out, seen: seen, opts: opts,
		perTicker: map[string]int{}, stats: map[string]*aliasStat{},
	}, nil
}

func (r *universeRun) emit(ticker, id, title, source string, weight float64, ts int64) error {
	if !wanted(ticker, r.opts.tickers) {
		return nil
	}
	k1 := ticker + "|" + id
	k2 := titleKey(ticker, title)
	if r.seen[k1] || r.seen[k2] {
		r.skipped++
		return nil
	}
	r.seen[k1] = true
	r.seen[k2] = true
	line, err := json.Marshal(record{Ticker: ticker, ArticleID: id, Title: title, Source: source, TrustWeight: weight, PublishedTS: ts})
	if err != nil {
		return err
	}
	if _, err := r.out.Write(append(line, '\n')); err != nil {
		return err
	}
	r.written++
	r.perTicker[ticker]++
	return nil
}

func rowTitle(row kasymRow) string {
	title := strings.TrimSpace(row.Title)
	if title != "" && !strings.EqualFold(title, noTitle) {
		return title
	}
	body := []rune(strings.TrimSpace(row.Body))
	if len(body) > maxBodyTitleRune {
		body = body[:maxBodyTitleRune]
	}
	return strings.TrimSpace(string(body))
}

func (r *universeRun) importParquetUniverse(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open parquet %q: %w", path, err)
	}
	defer f.Close()
	pr := parquet.NewGenericReader[kasymRow](f)
	defer pr.Close()
	before := r.written
	rows := make([]kasymRow, 1000)
	for {
		n, err := pr.Read(rows)
		for _, row := range rows[:n] {
			published, ok := parsePublished(row.Date, row.Time)
			if !ok {
				continue
			}
			title := rowTitle(row)
			if title == "" {
				continue
			}
			tags := parseTags(row.Tags)
			tagged := tagTickers(tags, r.tickerSet)
			titleHits := r.uni.Match(title)
			r.audit(tagged, titleHits)
			tickers := map[string]bool{}
			for _, t := range tagged {
				tickers[t] = true
			}
			for _, h := range titleHits {
				tickers[h.Ticker] = true
			}
			for _, tag := range tags {
				for _, h := range r.uni.Match(tag) {
					tickers[h.Ticker] = true
				}
			}
			id := hashID(row.Title, row.Body)
			sorted := make([]string, 0, len(tickers))
			for t := range tickers {
				sorted = append(sorted, t)
			}
			sort.Strings(sorted)
			for _, t := range sorted {
				if err := r.emit(t, id, title, row.Source, r.opts.trustWeight, published.Unix()); err != nil {
					return 0, err
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return 0, fmt.Errorf("read parquet %q rows: %w", path, err)
		}
		if n == 0 {
			break
		}
	}
	return r.written - before, nil
}

func (r *universeRun) audit(tagged []string, hits []news.Hit) {
	if len(tagged) == 0 {
		return
	}
	tagSet := map[string]bool{}
	for _, t := range tagged {
		tagSet[t] = true
	}
	hitSet := map[string]bool{}
	for _, h := range hits {
		hitSet[h.Ticker] = true
		key := h.Alias + "\t" + h.Ticker
		st := r.stats[key]
		if st == nil {
			st = &aliasStat{}
			r.stats[key] = st
		}
		st.matches++
		if tagSet[h.Ticker] {
			st.correct++
		}
	}
	for t := range tagSet {
		r.tagged++
		if hitSet[t] {
			r.recalled++
		}
	}
}

func (r *universeRun) writeAudit(path string) error {
	type row struct {
		key string
		st  *aliasStat
	}
	rows := make([]row, 0, len(r.stats))
	for k, st := range r.stats {
		rows = append(rows, row{k, st})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].st.matches > rows[j].st.matches })
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	fmt.Fprintln(w, "alias\tticker\tmatches_on_tagged_titles\tcorrect\tprecision")
	for _, rw := range rows {
		fmt.Fprintf(w, "%s\t%d\t%d\t%.3f\n", rw.key, rw.st.matches, rw.st.correct, float64(rw.st.correct)/float64(rw.st.matches))
	}
	return w.Flush()
}

type jsonlRecord struct {
	Ticker      string  `json:"ticker"`
	ArticleID   string  `json:"article_id"`
	Title       string  `json:"title"`
	Source      string  `json:"source"`
	TrustWeight float64 `json:"trust_weight"`
	PublishedTS int64   `json:"published_ts"`
}

func (r *universeRun) importJSONL(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	before := r.written
	for scanner.Scan() {
		var rec jsonlRecord
		if json.Unmarshal(scanner.Bytes(), &rec) != nil {
			continue
		}
		title := strings.TrimSpace(rec.Title)
		if title == "" || strings.EqualFold(title, noTitle) || rec.PublishedTS <= 0 {
			continue
		}
		weight := rec.TrustWeight
		if weight <= 0 {
			weight = r.opts.trustWeight
		}
		id := rec.ArticleID
		if id == "" {
			id = hashID(title, "")
		}
		tickers := map[string]bool{}
		for _, h := range r.uni.Match(title) {
			tickers[h.Ticker] = true
		}
		if strings.HasPrefix(rec.Source, "Google News:") && r.tickerSet[rec.Ticker] {
			tickers[rec.Ticker] = true
		}
		sorted := make([]string, 0, len(tickers))
		for t := range tickers {
			sorted = append(sorted, t)
		}
		sort.Strings(sorted)
		for _, t := range sorted {
			if err := r.emit(t, id, title, rec.Source, weight, rec.PublishedTS); err != nil {
				return 0, err
			}
		}
	}
	return r.written - before, scanner.Err()
}
