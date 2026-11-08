package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type futureMeta struct {
	SecID     string `json:"secid"`
	Lotsize   string `json:"lotsize"`
	Unit      string `json:"unit"`
	FaceUnit  string `json:"faceunit"`
	AssetCode string `json:"assetcode"`
	Name      string `json:"name"`
	LastTrade string `json:"lasttradedate"`
}

func metaMode(r *responder, dir string, workers int) error {
	entries, err := filepath.Glob(filepath.Join(dir, "futures", "*.csv.gz"))
	if err != nil {
		return err
	}
	var ids []string
	for _, e := range entries {
		ids = append(ids, strings.TrimSuffix(filepath.Base(e), ".csv.gz"))
	}
	sort.Strings(ids)
	work := make(chan string)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var out []futureMeta
	var done, failed int
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range work {
				t, err := r.get("/securities/"+id+".json", map[string][]string{"iss.meta": {"off"}, "iss.only": {"description"}})
				if err != nil {
					mu.Lock()
					failed++
					mu.Unlock()
					continue
				}
				d, ok := t["description"]
				if !ok {
					continue
				}
				nameIdx, valueIdx := d.index("name"), d.index("value")
				rec := map[string]string{}
				for _, row := range d.Data {
					rec[cell(row, nameIdx)] = cell(row, valueIdx)
				}
				mu.Lock()
				out = append(out, futureMeta{
					SecID: id, Lotsize: rec["LOTSIZE"], Unit: rec["UNIT"], FaceUnit: rec["FACEUNIT"],
					AssetCode: rec["ASSETCODE"], Name: rec["NAME"], LastTrade: rec["LSTTRADE"],
				})
				done++
				mu.Unlock()
			}
		}()
	}
	for _, id := range ids {
		work <- id
	}
	close(work)
	wg.Wait()
	sort.Slice(out, func(i, j int) bool { return out[i].SecID < out[j].SecID })
	f, err := os.Create(filepath.Join(dir, "futures-meta.jsonl"))
	if err != nil {
		return err
	}
	defer f.Close()
	for _, m := range out {
		b, err := json.Marshal(m)
		if err != nil {
			return err
		}
		if _, err := f.WriteString(string(b) + "\n"); err != nil {
			return err
		}
	}
	fmt.Fprintf(os.Stderr, "meta: %d written, %d failed\n", done, failed)
	return nil
}
