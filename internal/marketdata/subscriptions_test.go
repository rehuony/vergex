package marketdata

import (
	"slices"
	"testing"
	"time"

	"vergex/internal/provider/binance"
	"vergex/internal/provider/okx"
	"vergex/internal/testdb"
)

func subscriptionFixture(t *testing.T) (*subscriptions, *Store, *time.Time) {
	t.Helper()
	store, err := NewStore(testdb.Open(t))
	if err != nil {
		t.Fatal(err)
	}
	subs := newSubscriptions(store, binance.NewStream("ws://unused"))
	clock := time.UnixMilli(base())
	subs.now = func() time.Time { return clock }
	return subs, store, &clock
}

func TestStreamCarriesTheWatchlistButNotTheTopCoins(t *testing.T) {
	subs, store, _ := subscriptionFixture(t)
	if err := store.SaveSettings(true, []string{"BTCUSDT"}, 200); err != nil {
		t.Fatal(err)
	}
	subs.apply()
	want := []string{binance.AllPricesTopic, "btcusdt@kline_15m", "btcusdt@kline_1h", "btcusdt@kline_4h", "btcusdt@kline_1d"}
	if got := subs.binance.Topics(); !slices.Equal(got, want) {
		t.Fatalf("topics = %v, want %v", got, want)
	}
}

func TestStreamClosesWithNothingToFollow(t *testing.T) {
	subs, store, _ := subscriptionFixture(t)
	if err := store.SaveSettings(false, []string{"BTCUSDT"}, 0); err != nil {
		t.Fatal(err)
	}
	subs.apply()
	if got := subs.binance.Topics(); len(got) != 0 {
		t.Fatalf("topics = %v, want none", got)
	}
}

func TestLeasesComeBeforeTheWatchlistAndExpire(t *testing.T) {
	subs, store, clock := subscriptionFixture(t)
	if err := store.SaveSettings(true, []string{"BTCUSDT"}, 0); err != nil {
		t.Fatal(err)
	}
	subs.keep(subs.binance, binance.KlineTopic("SOLUSDT", "3m"), time.Hour)
	subs.keep(subs.binance, binance.KlineTopic("BTCUSDT", "1h"), time.Hour) // Also on the watchlist.
	subs.apply()
	got := subs.binance.Topics()
	if got[1] != "btcusdt@kline_1h" || got[2] != "solusdt@kline_3m" {
		t.Fatalf("topics = %v", got)
	}
	if count := len(got); count != 6 {
		t.Fatalf("topics = %v, a topic in both places should appear once", got)
	}

	*clock = clock.Add(time.Hour + time.Second)
	subs.apply()
	if slices.Contains(subs.binance.Topics(), "solusdt@kline_3m") {
		t.Fatal("an expired lease should be dropped")
	}
	if len(subs.leases) != 0 {
		t.Fatal("expired leases should be forgotten")
	}
}

func TestFollowersHoldATopicUntilTheLastLeaves(t *testing.T) {
	subs, store, clock := subscriptionFixture(t)
	if err := store.SaveSettings(false, nil, 0); err != nil {
		t.Fatal(err)
	}
	topic := binance.KlineTopic("ETHUSDT", "1m")
	first := subs.retain(subs.binance, topic)
	second := subs.retain(subs.binance, topic)
	subs.apply()
	if !slices.Contains(subs.binance.Topics(), topic) {
		t.Fatal("a followed topic should be subscribed")
	}

	first()
	first() // Closing twice must not release the other follower.
	*clock = clock.Add(time.Hour)
	subs.apply()
	if !slices.Contains(subs.binance.Topics(), topic) {
		t.Fatal("the second follower still needs it")
	}

	second()
	subs.apply()
	if got := subs.binance.Topics(); len(got) != 0 {
		t.Fatalf("topics = %v, want none once nobody follows", got)
	}
}

func TestOpeningASeriesSubscribesIt(t *testing.T) {
	store, err := NewStore(testdb.Open(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSettings(false, nil, 0); err != nil {
		t.Fatal(err)
	}
	service := NewService(store)
	if _, err := service.Open("ADAUSDT", "5m"); err != nil {
		t.Fatal(err)
	}
	service.subs.apply()
	if !slices.Contains(service.stream.Topics(), "adausdt@kline_5m") {
		t.Fatalf("topics = %v", service.stream.Topics())
	}
}

func TestPurgeRemovesOnlyOldShortCandles(t *testing.T) {
	store, err := NewStore(testdb.Open(t))
	if err != nil {
		t.Fatal(err)
	}
	short, _ := store.EnsureSeries("BTCUSDT", "1m", TierOnDemand, base())
	long, _ := store.EnsureSeries("BTCUSDT", "1h", TierOnDemand, base())
	for _, series := range []*Series{short, long} {
		if err := store.WriteCandlesForTest(series, []binance.Candle{closedBar(base()), closedBar(base() + 2*hour)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.purgeShortCandles(base() + hour); err != nil {
		t.Fatal(err)
	}
	if count, _ := store.CandleCount(short.ID); count != 1 {
		t.Fatalf("short candles = %d, want the newer one", count)
	}
	if count, _ := store.CandleCount(long.ID); count != 2 {
		t.Fatalf("long candles = %d, want both", count)
	}
}

func TestOKXStreamsCarryOnlyWhatWasAskedFor(t *testing.T) {
	store, err := NewStore(testdb.Open(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSettings(true, []string{"BTCUSDT"}, 0); err != nil {
		t.Fatal(err)
	}
	service := NewService(store)
	service.subs.apply()
	if got := service.okxCandles.Topics(); len(got) != 0 {
		t.Fatalf("okx candles = %v before anyone asked", got)
	}

	follower := service.FollowCandles(okx.Venue, "BTC-USDT-SWAP", "1h")
	service.WatchMark(okx.Venue, "ETHUSDT")
	service.subs.apply()
	if got := service.okxCandles.Topics(); !slices.Equal(got, []string{"candle1H:BTC-USDT-SWAP"}) {
		t.Fatalf("okx candles = %v", got)
	}
	if got := service.okxMarks.Topics(); !slices.Equal(got, []string{"mark-price:ETH-USDT-SWAP"}) {
		t.Fatalf("okx marks = %v", got)
	}

	follower.Close()
	service.subs.apply()
	if got := service.okxCandles.Topics(); len(got) != 0 {
		t.Fatalf("okx candles = %v after the chart closed", got)
	}
	if service.FollowCandles(okx.Venue, "BTC-USDT-SWAP", "1w") != nil {
		t.Fatal("an interval with no channel has nothing to follow")
	}
	if service.FollowCandles("nowhere", "BTCUSDT", "1h") != nil {
		t.Fatal("an unknown venue has nothing to follow")
	}
}
