package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/olegsidorkin/moex-trader/internal/ingestion/news"
)

const googlePrefix = "Google News: "

var tagBackedSources = map[string]bool{
	"bcs": true, "bcs_tech": true, "finam": true, "smart_lab": true, "rdv": true, "t_invest": true, "t_analytic": true,
}

type record struct {
	Ticker      string  `json:"ticker"`
	ArticleID   string  `json:"article_id"`
	Title       string  `json:"title"`
	Source      string  `json:"source"`
	TrustWeight float64 `json:"trust_weight"`
	PublishedTS int64   `json:"published_ts"`
}

type quarantined struct {
	record
	Reason string `json:"reason"`
}

type decision struct {
	keep   bool
	reason string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func decide(rec record, universe *news.Universe, known map[string]bool) decision {
	if strings.HasPrefix(rec.Source, googlePrefix) && strings.TrimPrefix(rec.Source, googlePrefix) == rec.Ticker {
		return decision{true, "feed_ticker"}
	}
	for _, h := range universe.Match(rec.Title) {
		if h.Ticker == rec.Ticker {
			return decision{true, "text_match"}
		}
	}
	if tagBackedSources[rec.Source] && known[rec.Ticker] {
		return decision{true, "tag_backed_source"}
	}
	if !known[rec.Ticker] {
		return decision{false, "ticker_outside_universe"}
	}
	return decision{false, "not_supported_by_text"}
}

func run(args []string) error {
	fs := flag.NewFlagSet("newsclean", flag.ContinueOnError)
	in := fs.String("in", "data/news_history.jsonl", "archive to clean")
	out := fs.String("out", "", "cleaned archive path (required)")
	quarantine := fs.String("quarantine", "", "file receiving removed pairs with a reason (required)")
	names := fs.String("names", "data/ticker_names.json", "ticker names JSON")
	blocklist := fs.String("alias-blocklist", "data/news_alias_blocklist.json", "alias blocklist JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" || *quarantine == "" {
		return fmt.Errorf("-out and -quarantine are required")
	}
	tickerNames, err := news.LoadTickerNames(*names)
	if err != nil {
		return err
	}
	blocked, err := loadBlocklist(*blocklist)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, n := range tickerNames {
		known[n.Ticker] = true
	}
	extra := map[string][]string{}
	for t, aliases := range news.DefaultAliases() {
		if known[t] {
			extra[t] = aliases
		}
	}
	universe := news.NewUniverse(news.UniverseConfig{Names: tickerNames, Extra: extra, Blocked: blocked})

	src, err := os.Open(*in)
	if err != nil {
		return err
	}
	defer src.Close()
	keepFile, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer keepFile.Close()
	dropFile, err := os.Create(*quarantine)
	if err != nil {
		return err
	}
	defer dropFile.Close()
	keepW, dropW := bufio.NewWriter(keepFile), bufio.NewWriter(dropFile)

	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	kept, dropped := 0, 0
	byReason := map[string]int{}
	bySourceDropped := map[string]int{}
	byTickerDropped := map[string]int{}
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		var rec record
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		d := decide(rec, universe, known)
		byReason[d.reason]++
		if d.keep {
			keepW.Write(line)
			keepW.WriteByte('\n')
			kept++
			continue
		}
		raw, err := json.Marshal(quarantined{record: rec, Reason: d.reason})
		if err != nil {
			return err
		}
		dropW.Write(raw)
		dropW.WriteByte('\n')
		dropped++
		bySourceDropped[rec.Source]++
		byTickerDropped[rec.Ticker]++
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if err := keepW.Flush(); err != nil {
		return err
	}
	if err := dropW.Flush(); err != nil {
		return err
	}
	fmt.Printf("kept %d, quarantined %d\n", kept, dropped)
	fmt.Println("decisions:", sortedCounts(byReason, 10))
	fmt.Println("quarantined by source:", sortedCounts(bySourceDropped, 10))
	fmt.Println("quarantined by ticker:", sortedCounts(byTickerDropped, 12))
	return nil
}

func sortedCounts(m map[string]int, limit int) string {
	type kv struct {
		k string
		v int
	}
	list := make([]kv, 0, len(m))
	for k, v := range m {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].v != list[j].v {
			return list[i].v > list[j].v
		}
		return list[i].k < list[j].k
	})
	if len(list) > limit {
		list = list[:limit]
	}
	parts := make([]string, len(list))
	for i, e := range list {
		parts[i] = fmt.Sprintf("%s=%d", e.k, e.v)
	}
	return strings.Join(parts, " ")
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
