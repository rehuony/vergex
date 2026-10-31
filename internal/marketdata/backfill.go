package marketdata

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"vergex/internal/provider/binance"

	"gorm.io/gorm/clause"
)

// Binance publishes history as files at data.binance.vision. They cost no API
// weight and reach back further than the REST endpoints, so deep history comes
// from them; only what the files do not cover yet is read over REST.
const defaultArchiveURL = "https://data.binance.vision/data/futures/um"

// maxArchiveBytes refuses a file far larger than any real monthly archive.
const maxArchiveBytes = 128 << 20

var errNoArchive = errors.New("no archive for this period")

// BackfillOptions says what to load.
type BackfillOptions struct {
	Symbols     []string `json:"symbols"`
	Intervals   []string `json:"intervals"`
	Months      int      `json:"months"`
	Derivatives bool     `json:"derivatives"`
}

// BackfillProgress reports a running or finished backfill.
type BackfillProgress struct {
	Running    bool   `json:"running"`
	Current    string `json:"current,omitempty"`
	Done       int    `json:"done"`
	Total      int    `json:"total"`
	Error      string `json:"error,omitempty"`
	FinishedAt int64  `json:"finished_at,omitempty"`
}

// Backfiller loads deep history. It runs only when asked, never on its own.
type Backfiller struct {
	store      *Store
	client     *binance.DataClient
	http       *http.Client
	archiveURL string
	now        func() time.Time
}

// NewBackfiller creates a backfiller.
func NewBackfiller(store *Store, client *binance.DataClient) *Backfiller {
	return &Backfiller{store: store, client: client, http: &http.Client{Timeout: 2 * time.Minute}, archiveURL: defaultArchiveURL, now: time.Now}
}

// Run loads the requested history, reporting progress as it goes. A period the
// exchange has no file for is skipped: the coin may not have been listed yet.
func (b *Backfiller) Run(ctx context.Context, options BackfillOptions, report func(BackfillProgress)) error {
	if options.Months <= 0 {
		options.Months = 12
	}
	months := completeMonths(b.now().UTC(), options.Months)
	progress := BackfillProgress{Running: true, Total: len(options.Symbols) * len(options.Intervals)}
	if options.Derivatives {
		progress.Total += len(options.Symbols)
	}
	report(progress)
	for _, symbol := range options.Symbols {
		for _, interval := range options.Intervals {
			progress.Current = symbol + " " + interval
			report(progress)
			if err := b.loadCandles(ctx, symbol, interval, months); err != nil {
				return fmt.Errorf("%s %s: %w", symbol, interval, err)
			}
			progress.Done++
			report(progress)
		}
		if options.Derivatives {
			progress.Current = symbol
			report(progress)
			if err := b.loadDerivatives(ctx, symbol, months); err != nil {
				return fmt.Errorf("%s derivatives: %w", symbol, err)
			}
			progress.Done++
			report(progress)
		}
	}
	return nil
}

// completeMonths lists the n whole months before now, oldest first, as 2006-01.
func completeMonths(now time.Time, n int) []string {
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	months := make([]string, 0, n)
	for i := n; i >= 1; i-- {
		months = append(months, first.AddDate(0, -i, 0).Format("2006-01"))
	}
	return months
}

func (b *Backfiller) loadCandles(ctx context.Context, symbol, interval string, months []string) error {
	step := IntervalMillis(interval)
	if step == 0 {
		return fmt.Errorf("unsupported interval %q", interval)
	}
	now := b.now().UnixMilli()
	series, err := b.store.EnsureSeries(symbol, interval, TierOnDemand, now)
	if err != nil {
		return err
	}
	var first, last int64
	for _, month := range months {
		path := fmt.Sprintf("monthly/klines/%s/%s/%s-%s-%s.zip", symbol, interval, symbol, interval, month)
		rows, err := b.fetchCSV(ctx, path)
		if errors.Is(err, errNoArchive) {
			continue
		}
		if err != nil {
			return fmt.Errorf("%s: %w", month, err)
		}
		candles, err := parseArchiveKlines(rows)
		if err != nil {
			return fmt.Errorf("%s: %w", month, err)
		}
		if len(candles) == 0 {
			continue
		}
		if err := insertCandles(b.store.db, series.ID, candles); err != nil {
			return err
		}
		if first == 0 {
			first = candles[0].OpenTime
		}
		last = candles[len(candles)-1].OpenTime
	}
	if first == 0 {
		return nil
	}
	current, err := b.store.FindSeries(symbol, interval)
	if err != nil {
		return err
	}
	if current.FirstOpenTime == 0 || first < current.FirstOpenTime {
		current.FirstOpenTime = first
	}
	// A series with no data yet continues from the files instead of starting a
	// separate recent stretch with a hole before it.
	if current.LastClosedTime == 0 || current.LastClosedTime < last {
		current.LastClosedTime = last
		current.NextDueAt = now
	}
	current.Priority = seriesPriority(current, now)
	current.UpdatedAt = now
	if err := b.store.saveSeries(current); err != nil {
		return err
	}
	return b.fillHoles(ctx, current)
}

// fillHoles finds places where stored candles do not join up, such as between
// the files and the recent stretch, and reads them over REST within the budget.
func (b *Backfiller) fillHoles(ctx context.Context, series *Series) error {
	step := IntervalMillis(series.Interval)
	gaps, err := b.store.Gaps(series.ID)
	if err != nil {
		return err
	}
	known := make(map[int64]bool, len(gaps))
	for _, gap := range gaps {
		known[gap.FromTime] = true
	}
	var times []int64
	err = b.store.db.Model(&Candle{}).Where("series_id = ?", series.ID).Order("open_time ASC").Pluck("open_time", &times).Error
	if err != nil {
		return err
	}
	for i := 1; i < len(times); i++ {
		expected := times[i-1] + step
		if times[i] == expected || known[expected] {
			continue
		}
		if err := b.fillHole(ctx, series, expected, times[i]); err != nil {
			return err
		}
	}
	return nil
}

func (b *Backfiller) fillHole(ctx context.Context, series *Series, from, until int64) error {
	step := IntervalMillis(series.Interval)
	expected := from
	for expected < until {
		rows, err := b.client.Klines(ctx, series.Symbol, series.Interval, expected, pageLimit)
		if err != nil {
			return err
		}
		rows = dropUnclosed(rows, step, b.now().UnixMilli())
		if len(rows) == 0 {
			return nil
		}
		// Whatever the exchange skips over here it does not have.
		if rows[0].OpenTime > expected {
			if err := b.addGap(series.ID, expected, min(rows[0].OpenTime, until)-step); err != nil {
				return err
			}
			expected = rows[0].OpenTime
			continue
		}
		keep, resume, err := contiguous(rows, expected, step)
		if err != nil {
			return err
		}
		if err := insertCandles(b.store.db, series.ID, keep); err != nil {
			return err
		}
		expected += int64(len(keep)) * step
		if resume > 0 {
			if err := b.addGap(series.ID, expected, min(resume, until)-step); err != nil {
				return err
			}
			expected = resume
		} else if len(keep) == 0 {
			return nil
		}
	}
	return nil
}

func (b *Backfiller) addGap(seriesID, from, to int64) error {
	return b.store.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&Gap{SeriesID: seriesID, FromTime: from, ToTime: to}).Error
}

func (b *Backfiller) loadDerivatives(ctx context.Context, symbol string, months []string) error {
	for _, month := range months {
		path := fmt.Sprintf("monthly/fundingRate/%s/%s-fundingRate-%s.zip", symbol, symbol, month)
		rows, err := b.fetchCSV(ctx, path)
		if errors.Is(err, errNoArchive) {
			continue
		}
		if err != nil {
			return fmt.Errorf("funding %s: %w", month, err)
		}
		if err := b.store.SaveDerivatives(symbol, KindFunding, parseArchiveFunding(rows)); err != nil {
			return err
		}
	}
	// Metrics come as one file a day, so a day already held is not fetched again.
	if len(months) == 0 {
		return nil
	}
	start, err := time.Parse("2006-01", months[0])
	if err != nil {
		return err
	}
	today := b.now().UTC().Truncate(24 * time.Hour)
	for day := start; day.Before(today); day = day.AddDate(0, 0, 1) {
		held, err := b.store.Derivatives(symbol, KindOpenValue, day.UnixMilli(), day.AddDate(0, 0, 1).UnixMilli()-1)
		if err != nil {
			return err
		}
		if len(held) >= 20 {
			continue
		}
		path := fmt.Sprintf("daily/metrics/%s/%s-metrics-%s.zip", symbol, symbol, day.Format("2006-01-02"))
		rows, err := b.fetchCSV(ctx, path)
		if errors.Is(err, errNoArchive) {
			continue
		}
		if err != nil {
			return fmt.Errorf("metrics %s: %w", day.Format("2006-01-02"), err)
		}
		openValue, longShort := parseArchiveMetrics(rows)
		if err := b.store.SaveDerivatives(symbol, KindOpenValue, openValue); err != nil {
			return err
		}
		if err := b.store.SaveDerivatives(symbol, KindLongShort, longShort); err != nil {
			return err
		}
	}
	return nil
}

// fetchCSV downloads an archive and its checksum, verifies them and returns the
// CSV rows. Everything stays in memory; nothing is unpacked to disk.
func (b *Backfiller) fetchCSV(ctx context.Context, path string) ([][]string, error) {
	body, err := b.download(ctx, b.archiveURL+"/"+path)
	if err != nil {
		return nil, err
	}
	sum, err := b.download(ctx, b.archiveURL+"/"+path+".CHECKSUM")
	if err != nil {
		return nil, fmt.Errorf("checksum: %w", err)
	}
	fields := strings.Fields(string(sum))
	if len(fields) == 0 {
		return nil, errors.New("empty checksum file")
	}
	actual := sha256.Sum256(body)
	if hex.EncodeToString(actual[:]) != fields[0] {
		return nil, errors.New("checksum does not match")
	}
	archive, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, err
	}
	if len(archive.File) == 0 {
		return nil, errors.New("empty archive")
	}
	file, err := archive.File[0].Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := csv.NewReader(io.LimitReader(file, maxArchiveBytes))
	reader.FieldsPerRecord = -1
	return reader.ReadAll()
}

func (b *Backfiller) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errNoArchive
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxArchiveBytes))
}

// parseArchiveKlines reads kline rows. Newer files start with a header and older
// ones do not, so a first field that is not a number is skipped.
func parseArchiveKlines(rows [][]string) ([]binance.Candle, error) {
	candles := make([]binance.Candle, 0, len(rows))
	for _, row := range rows {
		if len(row) < 11 {
			continue
		}
		openTime, err := strconv.ParseInt(row[0], 10, 64)
		if err != nil {
			continue
		}
		candle := binance.Candle{OpenTime: openTime}
		candle.Trades, err = strconv.ParseInt(row[8], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("trade count at %d: %w", openTime, err)
		}
		for _, field := range []struct {
			index int
			dest  *float64
		}{
			{1, &candle.Open}, {2, &candle.High}, {3, &candle.Low}, {4, &candle.Close},
			{5, &candle.Volume}, {7, &candle.QuoteVolume}, {9, &candle.TakerBuyVolume}, {10, &candle.TakerBuyQuote},
		} {
			if *field.dest, err = strconv.ParseFloat(row[field.index], 64); err != nil {
				return nil, fmt.Errorf("field %d at %d: %w", field.index, openTime, err)
			}
		}
		candles = append(candles, candle)
	}
	return candles, nil
}

// parseArchiveFunding reads calc_time, funding_interval_hours, last_funding_rate.
func parseArchiveFunding(rows [][]string) []binance.Point {
	points := make([]binance.Point, 0, len(rows))
	for _, row := range rows {
		if len(row) < 3 {
			continue
		}
		at, err := strconv.ParseInt(row[0], 10, 64)
		if err != nil {
			continue
		}
		if rate, err := strconv.ParseFloat(row[2], 64); err == nil {
			points = append(points, binance.Point{Time: at, Value: rate})
		}
	}
	return points
}

// parseArchiveMetrics reads the 5-minute metrics file down to hourly readings of
// open interest value (column 3) and the long/short account ratio (column 6).
func parseArchiveMetrics(rows [][]string) (openValue, longShort []binance.Point) {
	for _, row := range rows {
		if len(row) < 7 {
			continue
		}
		at, err := time.Parse("2006-01-02 15:04:05", row[0])
		if err != nil || at.Minute() != 0 {
			continue
		}
		if value, err := strconv.ParseFloat(row[3], 64); err == nil {
			openValue = append(openValue, binance.Point{Time: at.UnixMilli(), Value: value})
		}
		if value, err := strconv.ParseFloat(row[6], 64); err == nil {
			longShort = append(longShort, binance.Point{Time: at.UnixMilli(), Value: value})
		}
	}
	return openValue, longShort
}
