package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/macro"
)

type city struct {
	name     string
	lat, lon float64
}

var cities = []city{
	{"moscow", 55.75, 37.62}, {"spb", 59.93, 30.34}, {"novosibirsk", 55.03, 82.92}, {"yekaterinburg", 56.84, 60.60},
	{"kazan", 55.79, 49.12}, {"nnovgorod", 56.33, 44.00}, {"samara", 53.20, 50.15}, {"krasnoyarsk", 56.01, 92.85},
	{"rostov", 47.24, 39.71}, {"khabarovsk", 48.48, 135.08},
}

var yahooSymbols = []struct{ file, sym string }{
	{"cm_ng", "NG=F"}, {"cm_ttf", "TTF=F"}, {"cm_hg", "HG=F"}, {"cm_ali", "ALI=F"}, {"cm_hrc", "HRC=F"}, {"cm_si", "SI=F"},
	{"cm_pl", "PL=F"}, {"cm_pa", "PA=F"}, {"cm_zw", "ZW=F"}, {"cm_zc", "ZC=F"}, {"cm_zs", "ZS=F"}, {"cm_ura", "URA"},
}

var gdeltQueries = []struct{ file, query, mode string }{
	{"gd_sanc_tone", "russia sanctions", "timelinetone"},
	{"gd_energy_tone", "russia oil gas energy", "timelinetone"},
	{"gd_econ_tone", "russia economy", "timelinetone"},
	{"gd_vol", "russia", "timelinevol"},
}

func fetchWeather(dir string) error {
	for _, c := range cities {
		u := fmt.Sprintf("https://archive-api.open-meteo.com/v1/archive?latitude=%.2f&longitude=%.2f&start_date=2014-01-01&end_date=%s&daily=temperature_2m_mean,precipitation_sum&timezone=Europe%%2FMoscow", c.lat, c.lon, time.Now().AddDate(0, 0, -2).Format(dayFmt))
		b, err := getBody(u)
		if err != nil {
			return err
		}
		var r struct {
			Daily struct {
				Time []string   `json:"time"`
				T    []*float64 `json:"temperature_2m_mean"`
				P    []*float64 `json:"precipitation_sum"`
			} `json:"daily"`
		}
		if err := json.Unmarshal(b, &r); err != nil {
			return err
		}
		var ts, ps macro.Series
		for i, d := range r.Daily.Time {
			t, err := time.Parse(dayFmt, d)
			if err != nil {
				return err
			}
			if r.Daily.T[i] != nil {
				ts = append(ts, macro.Point{Date: t, Value: *r.Daily.T[i]})
			}
			if r.Daily.P[i] != nil {
				ps = append(ps, macro.Point{Date: t, Value: *r.Daily.P[i]})
			}
		}
		if err := macro.SaveCSV(dir+"/wx_"+c.name+"_t.csv", ts); err != nil {
			return err
		}
		if err := macro.SaveCSV(dir+"/wx_"+c.name+"_p.csv", ps); err != nil {
			return err
		}
		fmt.Printf("weather %-14s %5d temperature days, %5d precipitation days, last %s\n", c.name, len(ts), len(ps), ts[len(ts)-1].Date.Format(dayFmt))
		time.Sleep(2 * time.Second)
	}
	return nil
}

func fetchYahoo(dir string) error {
	for _, y := range yahooSymbols {
		b, err := getBody("https://query1.finance.yahoo.com/v8/finance/chart/" + url.PathEscape(y.sym) + "?range=9y&interval=1d")
		if err != nil {
			return err
		}
		var r struct {
			Chart struct {
				Result []struct {
					Timestamp  []int64 `json:"timestamp"`
					Indicators struct {
						Quote []struct {
							Close []*float64 `json:"close"`
						} `json:"quote"`
					} `json:"indicators"`
				} `json:"result"`
			} `json:"chart"`
		}
		if err := json.Unmarshal(b, &r); err != nil || len(r.Chart.Result) == 0 {
			return fmt.Errorf("yahoo %s: %v", y.sym, err)
		}
		res := r.Chart.Result[0]
		var s macro.Series
		for i, ts := range res.Timestamp {
			if i < len(res.Indicators.Quote[0].Close) && res.Indicators.Quote[0].Close[i] != nil {
				s = append(s, macro.Point{Date: time.Unix(ts, 0).UTC().Truncate(24 * time.Hour), Value: *res.Indicators.Quote[0].Close[i]})
			}
		}
		if err := macro.SaveCSV(dir+"/"+y.file+".csv", s.Sorted()); err != nil {
			return err
		}
		fmt.Printf("yahoo %-8s %5d points %s .. %s\n", y.sym, len(s), s[0].Date.Format(dayFmt), s[len(s)-1].Date.Format(dayFmt))
		time.Sleep(time.Second)
	}
	return nil
}

func fetchGdelt(dir string) error {
	for _, g := range gdeltQueries {
		u := "https://api.gdeltproject.org/api/v2/doc/doc?query=" + url.QueryEscape(g.query) + "&mode=" + g.mode + "&format=json&startdatetime=20190101000000&enddatetime=" + time.Now().AddDate(0, 0, -1).Format("20060102") + "000000"
		var s macro.Series
		for attempt := 0; attempt < 4; attempt++ {
			b, err := getBody(u)
			if err == nil {
				var r struct {
					Timeline []struct {
						Data []struct {
							Date  string  `json:"date"`
							Value float64 `json:"value"`
						} `json:"data"`
					} `json:"timeline"`
				}
				if json.Unmarshal(b, &r) == nil && len(r.Timeline) > 0 {
					for _, p := range r.Timeline[0].Data {
						t, err := time.Parse("20060102T150405Z", strings.TrimSpace(p.Date))
						if err == nil {
							s = append(s, macro.Point{Date: t, Value: p.Value})
						}
					}
				}
			}
			if len(s) > 100 {
				break
			}
			time.Sleep(10 * time.Second)
		}
		if len(s) == 0 {
			return fmt.Errorf("gdelt %s: empty", g.file)
		}
		if err := macro.SaveCSV(dir+"/"+g.file+".csv", s.Sorted()); err != nil {
			return err
		}
		fmt.Printf("gdelt %-16s %5d points %s .. %s\n", g.file, len(s), s[0].Date.Format(dayFmt), s[len(s)-1].Date.Format(dayFmt))
		time.Sleep(8 * time.Second)
	}
	return nil
}

func fetchWorld(dir string) error {
	if _, err := os.Stat(dir + "/wx_moscow_t.csv"); err != nil {
		if err := fetchWeather(dir); err != nil {
			return err
		}
	}
	if err := fetchYahoo(dir); err != nil {
		return err
	}
	return fetchGdelt(dir)
}
