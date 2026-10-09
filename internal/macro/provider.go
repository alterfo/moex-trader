package macro

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"
)

const (
	FileIMOEX   = "imoex.csv"
	FileCNYRUB  = "cnyrub.csv"
	FileBrent   = "brent.csv"
	FileKeyRate = "keyrate.csv"
)

var FeatureNames = []string{
	"imoex_ret_5d",
	"imoex_ret_21d",
	"imoex_dist_ma50_pct",
	"cnyrub_ret_5d",
	"cnyrub_ret_21d",
	"brent_ret_5d",
	"brent_ret_21d",
	"key_rate_pct",
	"key_rate_chg_90d",
}

type Features struct {
	ImoexRet5d    float64
	ImoexRet21d   float64
	ImoexDistMA50 float64
	CnyRubRet5d   float64
	CnyRubRet21d  float64
	BrentRet5d    float64
	BrentRet21d   float64
	KeyRatePct    float64
	KeyRateChg90d float64
}

type Provider struct {
	mu      sync.RWMutex
	imoex   Series
	cnyrub  Series
	brent   Series
	keyRate Series
}

func NewProvider(imoex, cnyrub, brent, keyRate Series) *Provider {
	return &Provider{imoex: imoex.Sorted(), cnyrub: cnyrub.Sorted(), brent: brent.Sorted(), keyRate: keyRate.Sorted()}
}

func LoadProvider(dir string) (*Provider, error) {
	load := func(name string) (Series, error) { return LoadCSV(filepath.Join(dir, name)) }
	imoex, err := load(FileIMOEX)
	if err != nil {
		return nil, err
	}
	cnyrub, err := load(FileCNYRUB)
	if err != nil {
		return nil, err
	}
	brent, err := load(FileBrent)
	if err != nil {
		return nil, err
	}
	keyRate, err := load(FileKeyRate)
	if err != nil {
		return nil, err
	}
	return NewProvider(imoex, cnyrub, brent, keyRate), nil
}

func (p *Provider) Save(dir string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	for name, s := range map[string]Series{FileIMOEX: p.imoex, FileCNYRUB: p.cnyrub, FileBrent: p.brent, FileKeyRate: p.keyRate} {
		if err := SaveCSV(filepath.Join(dir, name), s); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) Replace(imoex, cnyrub, brent, keyRate Series) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.imoex, p.cnyrub, p.brent, p.keyRate = imoex.Sorted(), cnyrub.Sorted(), brent.Sorted(), keyRate.Sorted()
}

func (p *Provider) Snapshot() (imoex, cnyrub, brent, keyRate Series) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.imoex, p.cnyrub, p.brent, p.keyRate
}

func (p *Provider) At(day time.Time, maxStale time.Duration) (Features, error) {
	imoex, cnyrub, brent, keyRate := p.Snapshot()
	var f Features
	var ok bool

	imoexPast := imoex.before(day)
	cnyrubPast := cnyrub.before(day)
	brentPast := brent.before(day)
	keyPast := keyRate.before(day)

	for name, s := range map[string]Series{"imoex": imoexPast, "cnyrub": cnyrubPast, "brent": brentPast, "keyrate": keyPast} {
		last, has := s.Last()
		if !has {
			return Features{}, fmt.Errorf("macro: no %s history before %s", name, Day(day).Format("2006-01-02"))
		}
		if maxStale > 0 && Day(day).Sub(last.Date) > maxStale {
			return Features{}, fmt.Errorf("macro: %s is stale, last point %s for %s", name, last.Date.Format("2006-01-02"), Day(day).Format("2006-01-02"))
		}
	}

	if f.ImoexRet5d, ok = imoexPast.returnPct(5); !ok {
		return Features{}, fmt.Errorf("macro: imoex history too short")
	}
	if f.ImoexRet21d, ok = imoexPast.returnPct(21); !ok {
		return Features{}, fmt.Errorf("macro: imoex history too short")
	}
	if f.ImoexDistMA50, ok = imoexPast.distMAPct(50); !ok {
		return Features{}, fmt.Errorf("macro: imoex history too short")
	}
	if f.CnyRubRet5d, ok = cnyrubPast.returnPct(5); !ok {
		return Features{}, fmt.Errorf("macro: cnyrub history too short")
	}
	if f.CnyRubRet21d, ok = cnyrubPast.returnPct(21); !ok {
		return Features{}, fmt.Errorf("macro: cnyrub history too short")
	}
	if f.BrentRet5d, ok = brentPast.returnPct(5); !ok {
		return Features{}, fmt.Errorf("macro: brent history too short")
	}
	if f.BrentRet21d, ok = brentPast.returnPct(21); !ok {
		return Features{}, fmt.Errorf("macro: brent history too short")
	}
	last, _ := keyPast.Last()
	f.KeyRatePct = last.Value
	earlier, ok := keyPast.valueOnOrBefore(last.Date.AddDate(0, 0, -90))
	if !ok {
		return Features{}, fmt.Errorf("macro: key rate history too short")
	}
	f.KeyRateChg90d = last.Value - earlier
	return f, nil
}

func (f Features) Values() []float64 {
	return []float64{f.ImoexRet5d, f.ImoexRet21d, f.ImoexDistMA50, f.CnyRubRet5d, f.CnyRubRet21d, f.BrentRet5d, f.BrentRet21d, f.KeyRatePct, f.KeyRateChg90d}
}
