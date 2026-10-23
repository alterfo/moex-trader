package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/olegsidorkin/moex-trader/internal/macro"
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

func getBody(u string) ([]byte, error) {
	var last error
	for a := 0; a < 3; a++ {
		resp, err := httpClient.Get(u)
		if err != nil {
			last = err
			time.Sleep(time.Duration(a+1) * time.Second)
			continue
		}
		b, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 {
			last = fmt.Errorf("%s: status %d err %v", u, resp.StatusCode, err)
			time.Sleep(time.Duration(a+1) * time.Second)
			continue
		}
		return b, nil
	}
	return nil, last
}

type issBlock struct {
	Columns []string        `json:"columns"`
	Data    [][]interface{} `json:"data"`
}

func issTable(u, name string) (*issBlock, error) {
	b, err := getBody(u)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	var blk issBlock
	if err := json.Unmarshal(m[name], &blk); err != nil {
		return nil, err
	}
	return &blk, nil
}

func col(b *issBlock, name string) int {
	for i, c := range b.Columns {
		if c == name {
			return i
		}
	}
	return -1
}

func num(v interface{}) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

func fetchCandles(base string, from string) (macro.Series, error) {
	var out macro.Series
	for start := 0; ; {
		u := base + "?iss.meta=off&interval=24&from=" + from + "&start=" + strconv.Itoa(start)
		blk, err := issTable(u, "candles")
		if err != nil {
			return nil, err
		}
		if len(blk.Data) == 0 {
			break
		}
		ci, bi := col(blk, "close"), col(blk, "begin")
		for _, r := range blk.Data {
			v, ok := num(r[ci])
			if !ok {
				continue
			}
			d, err := time.Parse("2006-01-02", strings.Fields(r[bi].(string))[0])
			if err != nil {
				return nil, err
			}
			out = append(out, macro.Point{Date: d, Value: v})
		}
		start += len(blk.Data)
	}
	return out.Sorted(), nil
}

func fetchFundValue(secs []string, from string) (macro.Series, error) {
	sum := map[string]float64{}
	for _, sec := range secs {
		for start := 0; ; {
			u := "https://iss.moex.com/iss/history/engines/stock/markets/shares/securities/" + sec + ".json?iss.meta=off&from=" + from + "&history.columns=BOARDID,TRADEDATE,VALUE&start=" + strconv.Itoa(start)
			blk, err := issTable(u, "history")
			if err != nil {
				return nil, err
			}
			if len(blk.Data) == 0 {
				break
			}
			for _, r := range blk.Data {
				board, _ := r[0].(string)
				if board != "TQTF" && board != "TQBR" {
					continue
				}
				if v, ok := num(r[2]); ok {
					sum[r[1].(string)] += v
				}
			}
			start += len(blk.Data)
		}
	}
	var out macro.Series
	for d, v := range sum {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return nil, err
		}
		out = append(out, macro.Point{Date: t, Value: v})
	}
	return out.Sorted(), nil
}

func fetchKp() (macro.Series, error) {
	b, err := getBody("https://kp.gfz-potsdam.de/app/files/Kp_ap_since_1932.txt")
	if err != nil {
		return nil, err
	}
	mx := map[string]float64{}
	sc := bufio.NewScanner(strings.NewReader(string(b)))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 9 {
			continue
		}
		y, _ := strconv.Atoi(f[0])
		if y < 2019 {
			continue
		}
		kp, err := strconv.ParseFloat(f[7], 64)
		if err != nil || kp < 0 {
			continue
		}
		d := f[0] + "-" + f[1] + "-" + f[2]
		if kp > mx[d] {
			mx[d] = kp
		}
	}
	var out macro.Series
	for d, v := range mx {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return nil, err
		}
		out = append(out, macro.Point{Date: t, Value: v})
	}
	return out.Sorted(), nil
}

func fetchAll(dir string) error {
	jobs := []struct {
		name string
		f    func() (macro.Series, error)
	}{
		{"gold", func() (macro.Series, error) {
			return fetchCandles("https://iss.moex.com/iss/engines/currency/markets/selt/boards/CETS/securities/GLDRUB_TOM/candles.json", "2019-12-01")
		}},
		{"rvi", func() (macro.Series, error) {
			return fetchCandles("https://iss.moex.com/iss/engines/stock/markets/index/securities/RVI/candles.json", "2019-12-01")
		}},
		{"fundvalue", func() (macro.Series, error) {
			return fetchFundValue([]string{"TMOS", "SBMX", "EQMX"}, "2019-12-01")
		}},
		{"kp", fetchKp},
	}
	for _, j := range jobs {
		s, err := j.f()
		if err != nil {
			return fmt.Errorf("%s: %w", j.name, err)
		}
		if err := macro.SaveCSV(dir+"/"+j.name+".csv", s); err != nil {
			return err
		}
		if len(s) == 0 {
			fmt.Fprintf(os.Stdout, "%-10s EMPTY\n", j.name)
			continue
		}
		fmt.Printf("%-10s %5d points %s .. %s\n", j.name, len(s), s[0].Date.Format("2006-01-02"), s[len(s)-1].Date.Format("2006-01-02"))
	}
	return nil
}
