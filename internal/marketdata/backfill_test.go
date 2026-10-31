package marketdata

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"vergex/internal/provider/binance"
)

func zipped(t *testing.T, name, content string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	file, err := archive.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func archiveKlines(from, to int64, header bool) string {
	var rows []string
	if header {
		rows = append(rows, "open_time,open,high,low,close,volume,close_time,quote_volume,count,taker_buy_volume,taker_buy_quote_volume,ignore")
	}
	for at := from; at <= to; at += hour {
		rows = append(rows, fmt.Sprintf("%d,100,110,90,105,5,%d,500,10,2,200,0", at, at+hour-1))
	}
	return strings.Join(rows, "\n")
}

func archiveServer(t *testing.T, files map[string][]byte, corrupt bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if name, ok := strings.CutSuffix(path, ".CHECKSUM"); ok {
			body, found := files[name]
			if !found {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			sum := sha256.Sum256(body)
			if corrupt {
				sum[0] ^= 0xff
			}
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), name)
			return
		}
		body, found := files[path]
		if !found {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestBackfillJoinsFilesToRecentDataOverREST(t *testing.T) {
	december := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	january := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	now := january + 40*hour + 1000

	exchange := &fakeExchange{now: now}
	exchange.fill(december, january+40*hour)
	store, h := backfillStore(t, exchange)

	// The recent stretch was fetched at startup, leaving a hole after the files.
	series, err := store.EnsureSeries("BTCUSDT", "1h", TierWatch, now)
	if err != nil {
		t.Fatal(err)
	}
	var recent []binance.Candle
	for at := january + 20*hour; at <= january+39*hour; at += hour {
		recent = append(recent, binance.Candle{OpenTime: at, Open: 1, High: 1, Low: 1, Close: 1})
	}
	series.FirstOpenTime = recent[0].OpenTime
	series.LastClosedTime = recent[len(recent)-1].OpenTime
	if err := store.appendCandles(series, recent, nil); err != nil {
		t.Fatal(err)
	}

	files := map[string][]byte{
		"monthly/klines/BTCUSDT/1h/BTCUSDT-1h-2025-12.zip": zipped(t, "x.csv", archiveKlines(december, january-hour, true)),
	}
	srv := archiveServer(t, files, false)
	b := NewBackfiller(store, h.client)
	b.archiveURL = srv.URL
	b.now = func() time.Time { return time.UnixMilli(now) }

	err = b.Run(context.Background(), BackfillOptions{Symbols: []string{"BTCUSDT"}, Intervals: []string{"1h"}, Months: 2}, func(BackfillProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := store.FindSeries("BTCUSDT", "1h")
	if got.FirstOpenTime != december {
		t.Fatal("first open time should reach back to the files")
	}
	if got.LastClosedTime != january+39*hour {
		t.Fatal("the water mark should not move back")
	}
	count, _ := store.CandleCount(got.ID)
	want := (january + 40*hour - december) / hour
	if count != want {
		t.Fatalf("stored %d, want %d", count, want)
	}
	if _, err := store.Window(got, january+39*hour, int(want)); err != nil {
		t.Fatalf("history should be continuous: %v", err)
	}
}

func TestBackfillStartsANewSeriesFromTheFiles(t *testing.T) {
	december := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC).UnixMilli()
	now := time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC).UnixMilli()
	exchange := &fakeExchange{now: now}
	store, h := backfillStore(t, exchange)
	files := map[string][]byte{
		"monthly/klines/NEWUSDT/1h/NEWUSDT-1h-2025-12.zip": zipped(t, "x.csv", archiveKlines(december, december+5*hour, false)),
	}
	srv := archiveServer(t, files, false)
	b := NewBackfiller(store, h.client)
	b.archiveURL = srv.URL
	b.now = func() time.Time { return time.UnixMilli(now) }
	if err := b.Run(context.Background(), BackfillOptions{Symbols: []string{"NEWUSDT"}, Intervals: []string{"1h"}, Months: 1}, func(BackfillProgress) {}); err != nil {
		t.Fatal(err)
	}
	got, _ := store.FindSeries("NEWUSDT", "1h")
	if got.LastClosedTime != december+5*hour {
		t.Fatalf("water mark = %d", got.LastClosedTime)
	}
	if count, _ := store.CandleCount(got.ID); count != 6 {
		t.Fatalf("stored %d", count)
	}
}

func TestBackfillRefusesAFileThatFailsItsChecksum(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC).UnixMilli()
	exchange := &fakeExchange{now: now}
	store, h := backfillStore(t, exchange)
	files := map[string][]byte{
		"monthly/klines/BTCUSDT/1h/BTCUSDT-1h-2025-12.zip": zipped(t, "x.csv", archiveKlines(1000*hour, 1001*hour, false)),
	}
	srv := archiveServer(t, files, true)
	b := NewBackfiller(store, h.client)
	b.archiveURL = srv.URL
	b.now = func() time.Time { return time.UnixMilli(now) }
	err := b.Run(context.Background(), BackfillOptions{Symbols: []string{"BTCUSDT"}, Intervals: []string{"1h"}, Months: 1}, func(BackfillProgress) {})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
	got, _ := store.FindSeries("BTCUSDT", "1h")
	if count, _ := store.CandleCount(got.ID); count != 0 {
		t.Fatalf("stored %d from a bad file", count)
	}
}

func TestArchiveMetricsKeepHourlyReadings(t *testing.T) {
	rows := [][]string{
		{"create_time", "symbol", "sum_open_interest", "sum_open_interest_value", "x", "y", "count_long_short_ratio", "z"},
		{"2026-09-15 00:00:00", "BTCUSDT", "1", "8088.5", "1", "2", "1.17", "1"},
		{"2026-09-15 00:05:00", "BTCUSDT", "1", "9999", "1", "2", "9", "1"},
	}
	openValue, longShort := parseArchiveMetrics(rows)
	if len(openValue) != 1 || openValue[0].Value != 8088.5 || len(longShort) != 1 || longShort[0].Value != 1.17 {
		t.Fatalf("got %v %v", openValue, longShort)
	}
}

func backfillStore(t *testing.T, exchange *fakeExchange) (*Store, *Hydrator) {
	t.Helper()
	h, store, _ := hydratorFixture(t, exchange)
	return store, h
}
