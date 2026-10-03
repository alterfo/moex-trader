package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	pb "github.com/tinkoff/invest-api-go-sdk/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	prodEndpoint    = "invest-public-api.tinkoff.ru:443"
	sandboxEndpoint = "sandbox-invest-public-api.tbank.ru:443"
)

func main() {
	sandboxTest := flag.Bool("sandbox", true, "run sandbox futures-order feasibility test")
	deposit := flag.String("deposit", "1000000", "sandbox pay-in amount")
	flag.Parse()

	token := strings.TrimSpace(os.Getenv("MOEX_TRADER_TINKOFF_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("T_TOKEN"))
	}
	if token == "" {
		fmt.Println("FATAL: no token in MOEX_TRADER_TINKOFF_TOKEN or T_TOKEN")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	prodConn, err := dial(ctx, prodEndpoint, token)
	if err != nil {
		fmt.Println("FATAL prod dial:", err)
		os.Exit(2)
	}
	defer prodConn.Close()
	instr := pb.NewInstrumentsServiceClient(prodConn)
	md := pb.NewMarketDataServiceClient(prodConn)

	fmt.Println("== INDEX FUTURES ==")
	futures, err := instr.Futures(ctx, &pb.InstrumentsRequest{
		InstrumentStatus: pb.InstrumentStatus_INSTRUMENT_STATUS_ALL,
	})
	if err != nil {
		fmt.Println("FATAL futures list:", err)
		os.Exit(2)
	}
	all := futures.GetInstruments()
	fmt.Printf("total futures listed: %d\n", len(all))

	byAsset := map[string]int{}
	for _, f := range all {
		byAsset[f.GetAssetType()]++
	}
	fmt.Println("asset_type distribution:", byAsset)

	var indexFutures []*pb.Future
	for _, f := range all {
		if strings.EqualFold(f.GetAssetType(), "index") || looksLikeIndex(f) {
			indexFutures = append(indexFutures, f)
		}
	}
	fmt.Printf("index-like futures: %d\n", len(indexFutures))
	for _, f := range indexFutures {
		printFuture(f)
	}

	if len(indexFutures) == 0 {
		fmt.Println("no index futures found")
	} else {
		probe := pickIMOEX(indexFutures)
		fmt.Printf("\n== MARGIN + CANDLES for %s (%s) basic=%q api=%v ==\n", probe.GetTicker(), probe.GetUid(), probe.GetBasicAsset(), probe.GetApiTradeAvailableFlag())
		margin, err := instr.GetFuturesMargin(ctx, &pb.GetFuturesMarginRequest{Figi: probe.GetFigi()})
		if err != nil {
			fmt.Println("margin error:", err)
		} else {
			fmt.Printf("initial_margin_on_buy=%s %s\n", mv(margin.GetInitialMarginOnBuy()), margin.GetInitialMarginOnBuy().GetCurrency())
			fmt.Printf("initial_margin_on_sell=%s %s\n", mv(margin.GetInitialMarginOnSell()), margin.GetInitialMarginOnSell().GetCurrency())
			fmt.Printf("min_price_increment=%s amount=%s\n", qt(margin.GetMinPriceIncrement()), qt(margin.GetMinPriceIncrementAmount()))
		}
		from := time.Now().AddDate(0, 0, -30)
		candles, err := md.GetCandles(ctx, &pb.GetCandlesRequest{
			InstrumentId: probe.GetUid(),
			Interval:     pb.CandleInterval_CANDLE_INTERVAL_DAY,
			From:         timestamppb.New(from),
			To:           timestamppb.New(time.Now()),
		})
		if err != nil {
			fmt.Println("candles error:", err)
		} else {
			cs := candles.GetCandles()
			fmt.Printf("daily candles (30d): %d\n", len(cs))
			if len(cs) > 0 {
				last := cs[len(cs)-1]
				fmt.Printf("last candle %s close=%s vol=%d\n", last.GetTime().AsTime().Format("2006-01-02"), qt(last.GetClose()), last.GetVolume())
			}
		}
		if *sandboxTest {
			controlUID := resolveShare(ctx, instr, "SBER")
			controlPrice := lastPrice(ctx, md, controlUID)
			futurePrice := lastPrice(ctx, md, probe.GetUid())
			fmt.Printf("control share SBER uid=%s last=%s\n", controlUID, controlPrice)
			fmt.Printf("futures %s last=%s\n", probe.GetTicker(), futurePrice)
			runSandbox(ctx, token, *deposit, probe, controlUID, controlPrice, futurePrice)
		}
	}
}

func runSandbox(ctx context.Context, token, deposit string, target *pb.Future, controlUID string, controlPrice, futurePrice decimal.Decimal) {
	fmt.Println("\n== SANDBOX FUTURES FEASIBILITY ==")
	sbConn, err := dial(ctx, sandboxEndpoint, token)
	if err != nil {
		fmt.Println("sandbox dial error:", err)
		return
	}
	defer sbConn.Close()
	sb := pb.NewSandboxServiceClient(sbConn)

	accounts, err := sb.GetSandboxAccounts(ctx, &pb.GetAccountsRequest{})
	if err != nil {
		fmt.Println("sandbox accounts error:", err)
		return
	}
	fmt.Printf("existing sandbox accounts: %d\n", len(accounts.GetAccounts()))

	opened, err := sb.OpenSandboxAccount(ctx, &pb.OpenSandboxAccountRequest{})
	if err != nil {
		fmt.Println("open sandbox account error:", err)
		return
	}
	acct := opened.GetAccountId()
	fmt.Printf("probe account opened: %s\n", acct)
	defer func() {
		if _, cerr := sb.CloseSandboxAccount(context.Background(), &pb.CloseSandboxAccountRequest{AccountId: acct}); cerr != nil {
			fmt.Println("close probe account error:", cerr)
		} else {
			fmt.Println("probe account closed")
		}
	}()

	payIn, _ := decimal.NewFromString(deposit)
	bal, err := sb.SandboxPayIn(ctx, &pb.SandboxPayInRequest{
		AccountId: acct,
		Amount:    rub(payIn),
	})
	if err != nil {
		fmt.Println("pay-in error:", err)
		return
	}
	fmt.Printf("pay-in balance: %s\n", mv(bal.GetBalance()))

	if controlUID != "" && controlPrice.IsPositive() {
		placeLimit(ctx, sb, acct, controlUID, controlPrice, "CONTROL SHARE SBER")
	}
	if futurePrice.IsPositive() {
		placeLimit(ctx, sb, acct, target.GetUid(), futurePrice, "FUTURES resting "+target.GetTicker())
		step := decimal.NewFromInt(25)
		marketable := futurePrice.Mul(decimal.NewFromFloat(1.02)).Div(step).Round(0).Mul(step)
		placeLimit(ctx, sb, acct, target.GetUid(), marketable, "FUTURES marketable "+target.GetTicker())
	}
}

func lastPrice(ctx context.Context, md pb.MarketDataServiceClient, uid string) decimal.Decimal {
	if uid == "" {
		return decimal.Zero
	}
	resp, err := md.GetLastPrices(ctx, &pb.GetLastPricesRequest{InstrumentId: []string{uid}})
	if err != nil {
		fmt.Println("last price error:", err)
		return decimal.Zero
	}
	for _, lp := range resp.GetLastPrices() {
		if lp.GetInstrumentUid() == uid {
			return decimal.NewFromInt(lp.GetPrice().GetUnits()).Add(decimal.NewFromInt(int64(lp.GetPrice().GetNano())).Div(decimal.NewFromInt(1000000000)))
		}
	}
	return decimal.Zero
}

func placeLimit(ctx context.Context, sb pb.SandboxServiceClient, acct, uid string, price decimal.Decimal, label string) {
	order := &pb.PostOrderRequest{
		InstrumentId: uid,
		AccountId:    acct,
		Quantity:     1,
		Direction:    pb.OrderDirection_ORDER_DIRECTION_BUY,
		OrderType:    pb.OrderType_ORDER_TYPE_LIMIT,
		Price:        quot(price),
		OrderId:      uuid.NewString(),
	}
	resp, err := sb.PostSandboxOrder(ctx, order)
	if err != nil {
		fmt.Printf("%s ORDER REJECTED: %v\n", label, err)
		return
	}
	fmt.Printf("%s ORDER ACCEPTED: status=%s lots_req=%d lots_exec=%d\n",
		label, resp.GetExecutionReportStatus(), resp.GetLotsRequested(), resp.GetLotsExecuted())
}

func resolveShare(ctx context.Context, instr pb.InstrumentsServiceClient, ticker string) string {
	resp, err := instr.FindInstrument(ctx, &pb.FindInstrumentRequest{Query: ticker, ApiTradeAvailableFlag: true})
	if err != nil {
		fmt.Println("find SBER error:", err)
		return ""
	}
	for _, i := range resp.GetInstruments() {
		if strings.EqualFold(i.GetTicker(), ticker) {
			return i.GetUid()
		}
	}
	return ""
}

func looksLikeIndex(f *pb.Future) bool {
	hay := strings.ToLower(f.GetTicker() + " " + f.GetName() + " " + f.GetBasicAsset())
	for _, k := range []string{"imoex", "индекс", "ртс", "rts", "мосбирж", "moex", "микс", "mix"} {
		if strings.Contains(hay, k) {
			return true
		}
	}
	return false
}

func printFuture(f *pb.Future) {
	fmt.Printf("ticker=%-8s uid=%s asset=%q basic=%q type=%s exp=%s api=%v buy=%v sell=%v short=%v weekend=%v lot=%d cur=%s\n",
		f.GetTicker(), f.GetUid(), f.GetAssetType(), f.GetBasicAsset(), f.GetFuturesType(),
		f.GetExpirationDate().AsTime().Format("2006-01-02"), f.GetApiTradeAvailableFlag(),
		f.GetBuyAvailableFlag(), f.GetSellAvailableFlag(), f.GetShortEnabledFlag(), f.GetWeekendFlag(),
		f.GetLot(), f.GetCurrency())
}

func pickIMOEX(fs []*pb.Future) *pb.Future {
	now := time.Now()
	var best *pb.Future
	for _, f := range fs {
		if !f.GetApiTradeAvailableFlag() {
			continue
		}
		if !strings.EqualFold(f.GetBasicAsset(), "IMOEX") {
			continue
		}
		if f.GetExpirationDate().AsTime().Before(now) {
			continue
		}
		if best == nil || f.GetExpirationDate().AsTime().Before(best.GetExpirationDate().AsTime()) {
			best = f
		}
	}
	if best != nil {
		return best
	}
	for _, f := range fs {
		if f.GetApiTradeAvailableFlag() && f.GetExpirationDate().AsTime().After(now) {
			if best == nil || f.GetExpirationDate().AsTime().Before(best.GetExpirationDate().AsTime()) {
				best = f
			}
		}
	}
	if best == nil {
		best = fs[0]
	}
	return best
}

func dial(ctx context.Context, endpoint, token string) (*grpc.ClientConn, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})),
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			return invoker(meta(ctx, token), method, req, reply, cc, opts...)
		}),
		grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return streamer(meta(ctx, token), desc, cc, method, opts...)
		}),
	}
	return grpc.DialContext(ctx, endpoint, opts...)
}

func meta(ctx context.Context, token string) context.Context {
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
	return metadata.AppendToOutgoingContext(ctx, "x-app-name", "moex-trader-probe")
}

func mv(v *pb.MoneyValue) string {
	if v == nil {
		return "nil"
	}
	return decimal.NewFromInt(v.GetUnits()).Add(decimal.NewFromInt(int64(v.GetNano())).Div(decimal.NewFromInt(1e9))).String()
}

func qt(v *pb.Quotation) string {
	if v == nil {
		return "nil"
	}
	return decimal.NewFromInt(v.GetUnits()).Add(decimal.NewFromInt(int64(v.GetNano())).Div(decimal.NewFromInt(1e9))).String()
}

func quot(d decimal.Decimal) *pb.Quotation {
	units := d.IntPart()
	nano := d.Sub(decimal.NewFromInt(units)).Mul(decimal.NewFromInt(1000000000)).Round(0).IntPart()
	return &pb.Quotation{Units: units, Nano: int32(nano)}
}

func rub(d decimal.Decimal) *pb.MoneyValue {
	units := d.IntPart()
	nano := d.Sub(decimal.NewFromInt(units)).Mul(decimal.NewFromInt(1000000000)).Round(0).IntPart()
	return &pb.MoneyValue{Currency: "rub", Units: units, Nano: int32(nano)}
}
