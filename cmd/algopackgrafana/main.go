package main

import (
	"bufio"
	"compress/gzip"
	"database/sql"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const mainSessionEnd = "18:50:00"

type dayRow struct {
	secid    string
	close    float64
	hasClose bool
	vol      float64
	val      float64
	trades   float64
	valB     float64
	valS     float64
	a2Sum    float64
	a2N      int
	a2Close  float64
	hasA2    bool
	spreadS  float64
	spreadN  int
}

func main() {
	root := flag.String("algopack", "data/algopack", "AlgoPack directory with tradestats/ and obstats/")
	imoexPath := flag.String("imoex", "data/macro-long/imoex.csv", "IMOEX close history (date,value)")
	ridgePath := flag.String("ridge", "data/ridge_scores.csv", "ridge score table (ticker,date,p)")
	out := flag.String("out", "data/grafana/algopack.db", "SQLite file for Grafana")
	refresh := flag.Int("refresh-days", 4, "re-read this many most recent stored days")
	workers := flag.Int("workers", 6, "parallel day readers")
	flag.Parse()

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", *out+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := initSchema(db); err != nil {
		log.Fatal(err)
	}

	have, err := storedDates(db)
	if err != nil {
		log.Fatal(err)
	}
	if *refresh > 0 && len(have) > 0 {
		cut := len(have) - *refresh
		if cut < 0 {
			cut = 0
		}
		for _, d := range have[cut:] {
			if _, err := db.Exec(`DELETE FROM day_raw WHERE day = ?`, d); err != nil {
				log.Fatal(err)
			}
		}
		have = have[:cut]
	}
	haveSet := map[string]bool{}
	for _, d := range have {
		haveSet[d] = true
	}

	dates, err := listDates(filepath.Join(*root, "tradestats"))
	if err != nil {
		log.Fatal(err)
	}
	var todo []string
	for _, d := range dates {
		if !haveSet[d] {
			todo = append(todo, d)
		}
	}
	log.Printf("days on disk %d, stored %d, to read %d", len(dates), len(have), len(todo))

	type result struct {
		date string
		rows []dayRow
		err  error
	}
	jobs := make(chan string)
	results := make(chan result)
	var wg sync.WaitGroup
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range jobs {
				rows, err := readDay(*root, d)
				results <- result{d, rows, err}
			}
		}()
	}
	go func() {
		for _, d := range todo {
			jobs <- d
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO day_raw (day, ts, secid, close, vol, val, trades, buy_share, a2, a2_close, spread_bbo) VALUES (?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		log.Fatal(err)
	}
	done := 0
	for r := range results {
		if r.err != nil {
			log.Printf("skip %s: %v", r.date, r.err)
			continue
		}
		t, err := time.Parse("2006-01-02", r.date)
		if err != nil {
			continue
		}
		ts := t.Unix()
		for _, w := range r.rows {
			if !w.hasClose {
				continue
			}
			var buy, a2, a2c, sp any
			if w.valB+w.valS > 0 {
				buy = w.valB / (w.valB + w.valS)
			}
			if w.a2N > 0 {
				a2 = w.a2Sum / float64(w.a2N)
			}
			if w.hasA2 {
				a2c = w.a2Close
			}
			if w.spreadN > 0 {
				sp = w.spreadS / float64(w.spreadN)
			}
			if _, err := stmt.Exec(r.date, ts, w.secid, w.close, w.vol, w.val, w.trades, buy, a2, a2c, sp); err != nil {
				log.Fatal(err)
			}
		}
		done++
		if done%100 == 0 {
			log.Printf("read %d/%d days", done, len(todo))
		}
	}
	if err := stmt.Close(); err != nil {
		log.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}

	if err := loadIMOEX(db, *imoexPath); err != nil {
		log.Fatal(err)
	}
	if err := loadRidge(db, *ridgePath); err != nil {
		log.Printf("ridge scores skipped: %v", err)
	}
	if err := rebuildDaily(db); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		log.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode=DELETE`); err != nil {
		log.Fatal(err)
	}
	var n, tk int
	_ = db.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT secid) FROM daily`).Scan(&n, &tk)
	log.Printf("daily rows %d, tickers %d, file %s", n, tk, *out)
}

func initSchema(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS day_raw (
			day TEXT NOT NULL, ts INTEGER NOT NULL, secid TEXT NOT NULL,
			close REAL, vol REAL, val REAL, trades REAL,
			buy_share REAL, a2 REAL, a2_close REAL, spread_bbo REAL,
			PRIMARY KEY (secid, day))`,
		`CREATE TABLE IF NOT EXISTS ridge (ts INTEGER NOT NULL, secid TEXT NOT NULL, p REAL NOT NULL, PRIMARY KEY (secid, ts))`,
		`CREATE TABLE IF NOT EXISTS imoex (day TEXT PRIMARY KEY, ts INTEGER NOT NULL, close REAL NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS daily (
			ts INTEGER NOT NULL, secid TEXT NOT NULL,
			close REAL, ret REAL, mret REAL,
			vol REAL, val REAL, trades REAL,
			buy_share REAL, a2 REAL, a2_close REAL, spread_bbo REAL,
			PRIMARY KEY (secid, ts))`,
		`CREATE INDEX IF NOT EXISTS daily_ts ON daily (ts)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}

func storedDates(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT DISTINCT day FROM day_raw ORDER BY day`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func listDates(dir string) ([]string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		name := e.Name()
		if strings.HasSuffix(name, ".csv.gz") {
			out = append(out, strings.TrimSuffix(name, ".csv.gz"))
		}
	}
	sort.Strings(out)
	return out, nil
}

func scanGz(path string, fn func(ix map[string]int, f []string)) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	gz, err := gzip.NewReader(fh)
	if err != nil {
		return err
	}
	defer gz.Close()
	r := csv.NewReader(bufio.NewReaderSize(gz, 1<<20))
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	ix := map[string]int{}
	for i, h := range head {
		ix[h] = i
	}
	for {
		f, err := r.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		fn(ix, f)
	}
}

func pf(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func readDay(root, date string) ([]dayRow, error) {
	byTicker := map[string]*dayRow{}
	lastClose := map[string]string{}
	lastA2 := map[string]string{}
	get := func(s string) *dayRow {
		w := byTicker[s]
		if w == nil {
			w = &dayRow{secid: s}
			byTicker[s] = w
		}
		return w
	}
	err := scanGz(filepath.Join(root, "tradestats", date+".csv.gz"), func(ix map[string]int, f []string) {
		w := get(f[ix["secid"]])
		tm := f[ix["tradetime"]]
		if v, ok := pf(f[ix["vol"]]); ok {
			w.vol += v
		}
		if v, ok := pf(f[ix["val"]]); ok {
			w.val += v
		}
		if v, ok := pf(f[ix["trades"]]); ok {
			w.trades += v
		}
		if v, ok := pf(f[ix["val_b"]]); ok {
			w.valB += v
		}
		if v, ok := pf(f[ix["val_s"]]); ok {
			w.valS += v
		}
		if tm < mainSessionEnd && tm >= lastClose[w.secid] {
			if v, ok := pf(f[ix["pr_close"]]); ok && v > 0 {
				w.close = v
				w.hasClose = true
				lastClose[w.secid] = tm
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("tradestats: %w", err)
	}
	err = scanGz(filepath.Join(root, "obstats", date+".csv.gz"), func(ix map[string]int, f []string) {
		w := byTicker[f[ix["secid"]]]
		if w == nil {
			return
		}
		tm := f[ix["tradetime"]]
		if v, ok := pf(f[ix["imbalance_val"]]); ok {
			w.a2Sum += v
			w.a2N++
			if tm < mainSessionEnd && tm >= lastA2[w.secid] {
				w.a2Close = v
				w.hasA2 = true
				lastA2[w.secid] = tm
			}
		}
		if v, ok := pf(f[ix["spread_bbo"]]); ok {
			w.spreadS += v
			w.spreadN++
		}
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("obstats: %w", err)
	}
	out := make([]dayRow, 0, len(byTicker))
	for _, w := range byTicker {
		out = append(out, *w)
	}
	return out, nil
}

func loadIMOEX(db *sql.DB, path string) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	r := csv.NewReader(fh)
	if _, err := r.Read(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO imoex (day, ts, close) VALUES (?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for {
		f, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		t, err := time.Parse("2006-01-02", f[0])
		if err != nil {
			continue
		}
		v, err := strconv.ParseFloat(f[1], 64)
		if err != nil || v <= 0 {
			continue
		}
		if _, err := stmt.Exec(f[0], t.Unix(), v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func loadRidge(db *sql.DB, path string) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	r := csv.NewReader(fh)
	if _, err := r.Read(); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM ridge`); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO ridge (ts, secid, p) VALUES (?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for {
		f, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		t, err := time.Parse("2006-01-02", f[1])
		if err != nil {
			continue
		}
		v, ok := pf(f[2])
		if !ok {
			continue
		}
		if _, err := stmt.Exec(t.Unix(), f[0], v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func rebuildDaily(db *sql.DB) error {
	if _, err := db.Exec(`DELETE FROM daily`); err != nil {
		return err
	}
	_, err := db.Exec(`
INSERT INTO daily (ts, secid, close, ret, mret, vol, val, trades, buy_share, a2, a2_close, spread_bbo)
SELECT ts, secid, close,
       close / prev_close - 1,
       mclose / mprev - 1,
       vol, val, trades, buy_share, a2, a2_close, spread_bbo
FROM (
  SELECT p.ts, p.secid, p.close, p.vol, p.val, p.trades, p.buy_share, p.a2, p.a2_close, p.spread_bbo,
         p.prev_close, m.close AS mclose, m2.close AS mprev
  FROM (
    SELECT r.*,
           LAG(r.close) OVER w AS prev_close,
           LAG(r.ts) OVER w AS prev_ts
    FROM day_raw r
    WINDOW w AS (PARTITION BY r.secid ORDER BY r.ts)
  ) p
  LEFT JOIN imoex m ON m.ts = p.ts
  LEFT JOIN imoex m2 ON m2.ts = p.prev_ts
)`)
	return err
}
