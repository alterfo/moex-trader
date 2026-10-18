package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"golang.org/x/net/proxy"

	"github.com/olegsidorkin/moex-trader/internal/alert/telegram"
)

const (
	requiredStreak = 5
	minHoldTrades  = 20
	ddSlackPP      = "0.5"
	minNetPnL      = "1"
)

type Metrics struct {
	Window   string          `json:"window"`
	Realized decimal.Decimal `json:"realized"`
	Net      decimal.Decimal `json:"net_mtm"`
	Trades   int             `json:"trades"`
	DDPct    decimal.Decimal `json:"max_dd_pct"`
}

type Day struct {
	Date      string             `json:"date"`
	Pass      bool               `json:"pass"`
	Reasons   []string           `json:"reasons"`
	Deployed  map[string]Metrics `json:"deployed"`
	Candidate map[string]Metrics `json:"candidate"`
}

type State struct {
	Days []Day `json:"days"`
}

var (
	reWindow    = regexp.MustCompile(`(?m)^- Window: (.+)$`)
	reRealized  = regexp.MustCompile(`(?m)^- Realized P&L[^:]*: \*\*(-?[0-9.]+) RUB\*\*`)
	reNetBorrow = regexp.MustCompile(`(?m)^- Realized P&L net of borrow: \*\*(-?[0-9.]+) RUB\*\*`)
	reNet       = regexp.MustCompile(`(?m)^- Net P&L \(MTM\): \*\*(-?[0-9.]+) RUB\*\*`)
	reTrades    = regexp.MustCompile(`(?m)^- Closed trades: ([0-9]+)`)
	reDD        = regexp.MustCompile(`(?m)^- Max drawdown: ([0-9.]+)%`)
)

func parseMetrics(md string) (Metrics, error) {
	var m Metrics
	get := func(re *regexp.Regexp) (string, error) {
		g := re.FindStringSubmatch(md)
		if g == nil {
			return "", fmt.Errorf("pattern %s not found", re.String())
		}
		return g[1], nil
	}
	w, err := get(reWindow)
	if err != nil {
		return m, err
	}
	m.Window = strings.TrimSpace(w)
	s, err := get(reRealized)
	if err != nil {
		return m, err
	}
	if m.Realized, err = decimal.NewFromString(s); err != nil {
		return m, err
	}
	if g := reNetBorrow.FindStringSubmatch(md); g != nil {
		if m.Realized, err = decimal.NewFromString(g[1]); err != nil {
			return m, err
		}
	}
	if s, err = get(reNet); err != nil {
		return m, err
	}
	if m.Net, err = decimal.NewFromString(s); err != nil {
		return m, err
	}
	if s, err = get(reTrades); err != nil {
		return m, err
	}
	fmt.Sscanf(s, "%d", &m.Trades)
	if s, err = get(reDD); err != nil {
		return m, err
	}
	if m.DDPct, err = decimal.NewFromString(s); err != nil {
		return m, err
	}
	return m, nil
}

func loadSet(dir, model string) (map[string]Metrics, error) {
	out := map[string]Metrics{}
	for _, w := range []string{"hold30", "pre90"} {
		b, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("bt-%s-%s.md", model, w)))
		if err != nil {
			return nil, err
		}
		m, err := parseMetrics(string(b))
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", model, w, err)
		}
		out[w] = m
	}
	return out, nil
}

func evaluate(dep, cand map[string]Metrics) (bool, []string) {
	var fails []string
	h, c := dep["hold30"], cand["hold30"]
	if c.Realized.LessThan(h.Realized) {
		fails = append(fails, "hold30: реализованный P&L кандидата ниже развёрнутой")
	}
	if c.Net.LessThan(h.Net) {
		fails = append(fails, "hold30: P&L с открытыми позициями у кандидата ниже")
	}
	if c.Trades < minHoldTrades {
		fails = append(fails, fmt.Sprintf("hold30: у кандидата меньше %d закрытых сделок", minHoldTrades))
	}
	hp, cp := dep["pre90"], cand["pre90"]
	if cp.Realized.LessThan(hp.Realized) {
		fails = append(fails, "pre90: реализованный P&L кандидата ниже развёрнутой")
	}
	if cp.Realized.LessThan(decimal.RequireFromString(minNetPnL)) {
		fails = append(fails, "pre90: реализованный P&L кандидата не проходит предстартовый порог")
	}
	if cp.DDPct.GreaterThan(hp.DDPct.Add(decimal.RequireFromString(ddSlackPP))) {
		fails = append(fails, "pre90: просадка кандидата выше развёрнутой больше чем на 0.5 п.п.")
	}
	return len(fails) == 0, fails
}

func streak(days []Day) int {
	n := 0
	for i := len(days) - 1; i >= 0; i-- {
		if !days[i].Pass {
			break
		}
		n++
	}
	return n
}

func rub(d decimal.Decimal) string {
	return d.StringFixed(0)
}

func line(name string, dep, cand Metrics) string {
	return fmt.Sprintf("%s (%s)\n  кандидат: реализ. %s, с открытыми %s, сделок %d, просадка %s%%\n  развёрнутая: реализ. %s, с открытыми %s, сделок %d, просадка %s%%",
		name, dep.Window, rub(cand.Realized), rub(cand.Net), cand.Trades, cand.DDPct.StringFixed(2),
		rub(dep.Realized), rub(dep.Net), dep.Trades, dep.DDPct.StringFixed(2))
}

func message(d Day, st int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Ночное переобучение ансамбля, данные до %s\nКандидат в каждом окне обучен только на данных до начала окна (проверка вне обучения); свежий кандидат до %s сохранён на боксе.\n\n", d.Date, d.Date)
	b.WriteString(line("hold30", d.Deployed["hold30"], d.Candidate["hold30"]))
	b.WriteString("\n\n")
	b.WriteString(line("pre90", d.Deployed["pre90"], d.Candidate["pre90"]))
	b.WriteString("\n\n")
	if d.Pass {
		fmt.Fprintf(&b, "Гейт сегодня пройден. Серия подряд: %d из %d.\n", st, requiredStreak)
	} else {
		b.WriteString("Гейт сегодня НЕ пройден:\n")
		for _, r := range d.Reasons {
			b.WriteString("  - " + r + "\n")
		}
		b.WriteString("Серия прохождений сброшена.\n")
	}
	if d.Pass && st >= requiredStreak {
		fmt.Fprintf(&b, "\nПРЕДЛАГАЮ ЗАМЕНУ МОДЕЛИ: кандидат прошёл оба окна %d дней подряд.\nПерекрытие окон между днями велико, дни не независимы; поэтому окончательное решение за вами. Сам ensemble_model.json не меняю.\nЕсли согласны, скажите «заменяем»: тогда ensemble_model.json, golden backtest, parity fixture, метрики, пересборка ./trader, перезапуск через вотчдог и oosfreeze.\n", st)
	} else {
		b.WriteString("\nЗамену не предлагаю.")
	}
	return b.String()
}

func readEnv(paths []string) map[string]string {
	out := map[string]string{}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			l := strings.TrimSpace(sc.Text())
			if i := strings.Index(l, "="); i > 0 && !strings.HasPrefix(l, "#") {
				out[l[:i]] = strings.Trim(l[i+1:], `"'`)
			}
		}
		f.Close()
	}
	return out
}

func httpClient(proxyURL string) *http.Client {
	tr := &http.Transport{Proxy: nil}
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			if d, err := proxy.FromURL(u, proxy.Direct); err == nil {
				tr.DialContext = func(ctx context.Context, network, addr string) (conn netConn, err error) {
					return dialVia(ctx, d, network, addr)
				}
			}
		}
	}
	return &http.Client{Timeout: 20 * time.Second, Transport: tr}
}

func main() {
	dir := flag.String("dir", "", "")
	date := flag.String("date", "", "")
	statePath := flag.String("state", "", "")
	envs := flag.String("env", ".env,watchdog.env", "")
	proxyURL := flag.String("proxy", "socks5://127.0.0.1:3333", "")
	dry := flag.Bool("dry", false, "")
	flag.Parse()
	if *dir == "" || *date == "" || *statePath == "" {
		fmt.Fprintln(os.Stderr, "need -dir -date -state")
		os.Exit(2)
	}
	dep, err := loadSet(*dir, "deployed")
	check(err)
	cand, err := loadSet(*dir, "candidate")
	check(err)
	pass, reasons := evaluate(dep, cand)
	day := Day{Date: *date, Pass: pass, Reasons: reasons, Deployed: dep, Candidate: cand}

	var st State
	if b, err := os.ReadFile(*statePath); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	replaced := false
	for i := range st.Days {
		if st.Days[i].Date == day.Date {
			st.Days[i] = day
			replaced = true
		}
	}
	if !replaced {
		st.Days = append(st.Days, day)
	}
	sort.Slice(st.Days, func(i, j int) bool { return st.Days[i].Date < st.Days[j].Date })
	sb, _ := json.MarshalIndent(st, "", "  ")
	check(os.MkdirAll(filepath.Dir(*statePath), 0o755))
	check(os.WriteFile(*statePath, sb, 0o644))

	msg := message(day, streak(st.Days))
	fmt.Println(msg)
	if *dry {
		return
	}
	env := readEnv(strings.Split(*envs, ","))
	tok, chat := env["MOEX_TRADER_TELEGRAM_BOT_TOKEN"], env["MOEX_TRADER_TELEGRAM_CHAT_ID"]
	cl := telegram.New(tok, chat, httpClient(*proxyURL))
	if !cl.Enabled() {
		fmt.Fprintln(os.Stderr, "telegram token or chat id missing")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := cl.Send(ctx, msg); err != nil {
		if err2 := telegram.New(tok, chat, httpClient("")).Send(ctx, msg); err2 != nil {
			fmt.Fprintln(os.Stderr, "send failed:", err, "| direct:", err2)
			os.Exit(1)
		}
	}
	fmt.Fprintln(os.Stderr, "telegram: sent")
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
