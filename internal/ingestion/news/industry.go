package news

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/shopspring/decimal"
)

const (
	shortAliasRunes       = 4
	industryTrustDiscount = 0.5
)

type IndustryRule struct {
	Name     string
	Keywords []string
	Tickers  []string
}

func DefaultIndustryRules() []IndustryRule {
	return []IndustryRule{
		{
			Name:     "oil_products",
			Keywords: []string{"дизел", "нефтепродукт", "бензин", "мазут", "экспорт нефти", "нефтян", "потолок цен на нефть", "opec", "опек"},
			Tickers:  []string{"ROSN", "LKOH", "TATN", "SIBN", "SNGSP"},
		},
		{
			Name:     "gas",
			Keywords: []string{"спг", "lng", "газопровод", "северный поток", "экспорт газа", "поставки газа"},
			Tickers:  []string{"GAZP", "NVTK"},
		},
		{
			Name:     "metals",
			Keywords: []string{"пошлины на сталь", "экспорт стали", "алюмини", "никел", "палладий"},
			Tickers:  []string{"GMKN", "RUAL", "CHMF", "NLMK", "MAGN"},
		},
		{
			Name:     "banks_policy",
			Keywords: []string{"ключевая ставка", "ключевую ставку", "ключевой ставки"},
			Tickers:  []string{"SBER", "VTBR", "T", "SVCB"},
		},
	}
}

func (m *Matcher) matchIndustry(article Article, haystack string, direct []MatchedArticle) []MatchedArticle {
	if len(m.industryRules) == 0 {
		return nil
	}
	covered := make(map[string]struct{}, len(direct))
	for _, d := range direct {
		if d.Link == article.Link && d.Title == article.Title {
			covered[d.Ticker] = struct{}{}
		}
	}
	trust := article.Source.TrustWeight.Mul(decimal.NewFromFloat(industryTrustDiscount))
	var out []MatchedArticle
	for _, rule := range m.industryRules {
		if !containsAny(haystack, rule.Keywords) {
			continue
		}
		for _, ticker := range rule.Tickers {
			if _, ok := covered[ticker]; ok {
				continue
			}
			covered[ticker] = struct{}{}
			out = append(out, MatchedArticle{
				Ticker:      ticker,
				Title:       article.Title,
				Link:        article.Link,
				Description: article.Description,
				PublishedAt: article.PublishedAt,
				SourceName:  article.Source.Name,
				TrustWeight: trust,
				Indirect:    true,
			})
		}
	}
	return out
}

func containsAny(haystack string, keywords []string) bool {
	for _, keyword := range keywords {
		if containsAlias(haystack, strings.ToLower(keyword)) {
			return true
		}
	}
	return false
}

func containsAlias(haystack, alias string) bool {
	if alias == "" {
		return false
	}
	wholeWord := utf8.RuneCountInString(alias) <= shortAliasRunes
	for from := 0; from <= len(haystack)-len(alias); {
		idx := strings.Index(haystack[from:], alias)
		if idx < 0 {
			return false
		}
		start := from + idx
		end := start + len(alias)
		if startsWord(haystack, start) && (!wholeWord || endsWord(haystack, end)) {
			return true
		}
		from = start + 1
	}
	return false
}

func startsWord(s string, i int) bool {
	if i == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(s[:i])
	return !isWordRune(r)
}

func endsWord(s string, i int) bool {
	if i >= len(s) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	return !isWordRune(r)
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
