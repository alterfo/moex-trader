package model

import "github.com/olegsidorkin/moex-trader/internal/domain"

var defaultFeatureOrder = []string{
	"return_pct",
	"realized_volatility",
	"news_sentiment",
	"news_count",
	"order_book_imbalance",
	"mom_5d",
	"mom_21d",
	"mom_63d",
	"reversal_1d",
	"rsi_14",
	"dist_ma20_pct",
	"dist_ma50_pct",
	"realized_vol_21d_annualized_pct",
	"volume_zscore_20d",
}

func ToVector(f domain.FeatureContext) ([]float64, []string) {
	values := []float64{
		f.ReturnPct.InexactFloat64(),
		f.RealizedVolatility.InexactFloat64(),
		f.NewsSentiment.InexactFloat64(),
		float64(f.NewsCount),
		f.OrderBookImbalance.InexactFloat64(),
		f.Mom5d.InexactFloat64(),
		f.Mom21d.InexactFloat64(),
		f.Mom63d.InexactFloat64(),
		f.Reversal1d.InexactFloat64(),
		f.RSI14.InexactFloat64(),
		f.DistMA20Pct.InexactFloat64(),
		f.DistMA50Pct.InexactFloat64(),
		f.RealizedVol21d.InexactFloat64(),
		f.VolumeZScore20d.InexactFloat64(),
	}
	return values, append([]string(nil), defaultFeatureOrder...)
}
