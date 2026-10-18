package news

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"
)

type TickerName struct {
	Ticker    string `json:"ticker"`
	ShortName string `json:"short_name"`
	SecName   string `json:"sec_name"`
	LatName   string `json:"lat_name"`
}

type tickerNameFile struct {
	Tickers []TickerName `json:"tickers"`
}

func LoadTickerNames(path string) ([]TickerName, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file tickerNameFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("news: parse ticker names %q: %w", path, err)
	}
	return file.Tickers, nil
}

type UniverseConfig struct {
	Names   []TickerName
	Extra   map[string][]string
	Blocked map[string]bool
}

type Hit struct {
	Ticker string
	Alias  string
}

type phraseAlias struct {
	words   []string
	stems   []string
	tickers []string
	text    string
}

type Universe struct {
	words    map[string][]string
	stems    map[string][]string
	phrases  map[string][]phraseAlias
	symbols  map[string]string
	hashtags map[string]string
	tickers  []string
	aliases  int
}

var nameNoise = map[string]bool{
	"ао": true, "ап": true, "пао": true, "мкпао": true, "ипао": true, "оао": true, "зао": true, "ооо": true,
	"п": true, "пр": true, "прив": true, "ордин": true, "об": true, "вып": true, "акции": true, "адр": true,
	"pref": true, "ord": true, "shs": true, "ipjsc": true, "pjsc": true, "jsc": true, "ao": true, "ap": true,
	"3ап": true, "2ап": true, "1ап": true, "ап3": true, "и": true,
}

var genericAlias = map[string]bool{
	"банк": true, "система": true, "россети": true, "россия": true, "россии": true, "российские": true,
	"русская": true, "московская": true, "биржа": true, "завод": true, "нефть": true, "газ": true, "сталь": true,
	"энерго": true, "холдинг": true, "группа": true, "компания": true, "центр": true, "волга": true,
	"сибирь": true, "урал": true, "мир": true, "каршеринг": true, "ранее": true, "бывш": true,
	"тгк": true, "мрск": true, "фск": true, "юг": true, "север": true, "запад": true, "восток": true,
}

var symbolBlocked = map[string]bool{"MOEX": true, "DATA": true, "IMOEX": true}

var caseSuffixes = map[string]bool{
	"": true, "а": true, "у": true, "ы": true, "и": true, "е": true, "я": true, "ю": true,
	"ом": true, "ем": true, "ой": true, "ей": true, "ов": true, "ам": true, "ах": true, "ами": true,
}

func tokenize(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur = append(cur, r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func cleanNameTokens(name string) []string {
	var out []string
	for _, t := range tokenize(name) {
		rs := []rune(t)
		if len(rs) > 2 && rs[0] == 'i' && rs[1] > 127 {
			t = string(rs[1:])
		}
		if nameNoise[t] {
			continue
		}
		out = append(out, t)
	}
	return out
}

func stemOf(alias string) string {
	rs := []rune(alias)
	if len(rs) >= 5 {
		switch rs[len(rs)-1] {
		case 'а', 'я', 'у', 'ю', 'ы', 'и', 'е', 'о', 'ь', 'й':
			return string(rs[:len(rs)-1])
		}
	}
	return alias
}

func p2base(ticker string) string {
	if strings.HasSuffix(ticker, "P") && len(ticker) > 3 {
		return strings.TrimSuffix(ticker, "P")
	}
	return ticker
}

func sameCompany(tickers []string) bool {
	base := p2base(tickers[0])
	for _, t := range tickers[1:] {
		if p2base(t) != base {
			return false
		}
	}
	return true
}

const minFirstTokenRunes = 6

var adjectiveEndings = []string{"ый", "ий", "ой", "ая", "яя", "ое", "ее", "ые", "ие", "ых", "их", "ого", "его"}

func adjectiveLike(token string) bool {
	for _, e := range adjectiveEndings {
		if strings.HasSuffix(token, e) {
			return true
		}
	}
	return false
}

func derivePhrases(n TickerName) []string {
	var phrases []string
	for _, name := range []string{n.ShortName, n.SecName, n.LatName} {
		tokens := cleanNameTokens(name)
		if len(tokens) == 0 {
			continue
		}
		phrases = append(phrases, strings.Join(tokens, " "))
		if len(tokens) > 1 && len([]rune(tokens[0])) >= minFirstTokenRunes && !adjectiveLike(tokens[0]) {
			phrases = append(phrases, tokens[0])
		}
	}
	return phrases
}

func NewUniverse(cfg UniverseConfig) *Universe {
	raw := map[string]map[string]bool{}
	add := func(ticker, alias string) {
		alias = strings.Join(tokenize(alias), " ")
		if alias == "" || cfg.Blocked[alias] {
			return
		}
		if !strings.Contains(alias, " ") {
			if genericAlias[alias] || len([]rune(alias)) < 3 {
				return
			}
		}
		if raw[alias] == nil {
			raw[alias] = map[string]bool{}
		}
		raw[alias][ticker] = true
	}
	u := &Universe{
		words:    map[string][]string{},
		stems:    map[string][]string{},
		phrases:  map[string][]phraseAlias{},
		symbols:  map[string]string{},
		hashtags: map[string]string{},
	}
	seenTicker := map[string]bool{}
	for _, n := range cfg.Names {
		if n.Ticker == "" {
			continue
		}
		if !seenTicker[n.Ticker] {
			seenTicker[n.Ticker] = true
			u.tickers = append(u.tickers, n.Ticker)
		}
		for _, p := range derivePhrases(n) {
			add(n.Ticker, p)
		}
	}
	for ticker, aliases := range cfg.Extra {
		if !seenTicker[ticker] {
			seenTicker[ticker] = true
			u.tickers = append(u.tickers, ticker)
		}
		for _, a := range aliases {
			add(ticker, a)
		}
	}
	sort.Strings(u.tickers)
	for _, t := range u.tickers {
		lower := strings.ToLower(t)
		u.hashtags[lower] = t
		if len(t) >= 3 && !symbolBlocked[t] {
			u.symbols[t] = t
		}
	}
	for alias, set := range raw {
		tickers := make([]string, 0, len(set))
		for t := range set {
			tickers = append(tickers, t)
		}
		sort.Strings(tickers)
		if len(tickers) > 1 && !sameCompany(tickers) {
			continue
		}
		u.aliases++
		words := strings.Fields(alias)
		if len(words) == 1 {
			u.words[alias] = tickers
			stem := stemOf(alias)
			if len([]rune(stem)) >= 4 {
				u.stems[stem] = tickers
			}
			continue
		}
		stems := make([]string, len(words))
		for i, w := range words {
			stems[i] = stemOf(w)
		}
		u.phrases[words[0]] = append(u.phrases[words[0]], phraseAlias{words: words, stems: stems, tickers: tickers, text: alias})
		first := stemOf(words[0])
		if first != words[0] {
			u.phrases[first] = append(u.phrases[first], phraseAlias{words: words, stems: stems, tickers: tickers, text: alias})
		}
	}
	return u
}

func (u *Universe) Tickers() []string {
	return append([]string(nil), u.tickers...)
}

func (u *Universe) AliasCount() int {
	return u.aliases
}

func stemMatches(token, stem string) bool {
	if !strings.HasPrefix(token, stem) {
		return false
	}
	return caseSuffixes[token[len(stem):]]
}

func (u *Universe) tokenTickers(token string) ([]string, string) {
	if t, ok := u.words[token]; ok {
		return t, token
	}
	rs := []rune(token)
	for cut := 0; cut <= 3 && len(rs)-cut >= 4; cut++ {
		stem := string(rs[:len(rs)-cut])
		if t, ok := u.stems[stem]; ok && stemMatches(token, stem) {
			return t, stem
		}
	}
	return nil, ""
}

func (u *Universe) Match(text string) []Hit {
	var hits []Hit
	seen := map[string]bool{}
	record := func(ticker, alias string) {
		if !seen[ticker] {
			seen[ticker] = true
			hits = append(hits, Hit{Ticker: ticker, Alias: alias})
		}
	}
	for _, raw := range strings.FieldsFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("«»\"'()[]{}<>,.;:!?—–|/\\", r)
	}) {
		prefixed := strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "$")
		trimmed := strings.TrimLeft(raw, "#$")
		if prefixed {
			if t, ok := u.hashtags[strings.ToLower(trimmed)]; ok {
				record(t, "#"+t)
			}
			continue
		}
		if t, ok := u.symbols[trimmed]; ok {
			record(t, t)
		}
	}
	tokens := tokenize(text)
	for i, tok := range tokens {
		if tickers, alias := u.tokenTickers(tok); tickers != nil {
			for _, t := range tickers {
				record(t, alias)
			}
		}
		for _, cand := range u.phrases[tok] {
			if i+len(cand.words) > len(tokens) {
				continue
			}
			matched := true
			for k, w := range cand.words {
				got := tokens[i+k]
				if got != w && !stemMatches(got, cand.stems[k]) {
					matched = false
					break
				}
			}
			if matched {
				for _, t := range cand.tickers {
					record(t, cand.text)
				}
			}
		}
	}
	sort.Slice(hits, func(a, b int) bool { return hits[a].Ticker < hits[b].Ticker })
	return hits
}
