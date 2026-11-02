package forecast

import (
	"errors"
	"math"
	"sort"

	"vergex/internal/market"
)

// WindowBars is how many candles a forecast reads. Live forecasts and backtests
// pass the same window, so a candle always gets the same answer.
const WindowBars = 200

// MinBars is the least Evaluate can read. Forecasts always pass WindowBars, so a coin
// needs that much history to get one.
const MinBars = 120

// ErrNotEnoughHistory says a coin is too new to forecast.
var ErrNotEnoughHistory = errors.New("not enough history")

// ErrNothingToRead says the candles hold nothing to score, such as a coin that has not
// traded at all.
var ErrNothingToRead = errors.New("nothing to read in these candles")

// Directions.
const (
	DirectionUp      = "up"
	DirectionDown    = "down"
	DirectionNeutral = "neutral"
)

// Regimes.
const (
	RegimeTrending = "trending"
	RegimeRanging  = "ranging"
)

// Derivatives is what derivatives data says at the moment of a forecast. A nil
// field means no reading recent enough, and that factor is left out.
type Derivatives struct {
	Funding     *float64 `json:"funding"`       // Latest settled funding rate.
	OIChangePct *float64 `json:"oi_change_pct"` // Open interest change over the lookback, in percent.
	LongShortZ  *float64 `json:"long_short_z"`  // Long/short ratio against its own recent range.
}

// FactorValue is one factor's reading and what it added to the score.
type FactorValue struct {
	Name         string  `json:"name"`
	Value        float64 `json:"value"`  // From -1 (down) to 1 (up).
	Weight       float64 `json:"weight"` // The weight used in this regime.
	Contribution float64 `json:"contribution"`
}

// Result is one forecast.
type Result struct {
	Close float64 `json:"close"`
	// ProbCandles uses only candle factors; ProbFull adds derivatives.
	ProbCandles float64 `json:"prob_candles"`
	ProbFull    float64 `json:"prob_full"`
	Direction   string  `json:"direction"`
	Confidence  int     `json:"confidence"`
	// ExpectedMovePct is the size of a typical move over the horizon.
	ExpectedMovePct float64 `json:"expected_move_pct"`
	Support         float64 `json:"support"`
	Resistance      float64 `json:"resistance"`
	Regime          string  `json:"regime"`
	ADX             float64 `json:"adx"`
	Horizon         int     `json:"horizon"`
	HasDerivatives  bool    `json:"has_derivatives"`
	// Derivatives is what the derivatives factors read.
	Derivatives Derivatives   `json:"derivatives"`
	Factors     []FactorValue `json:"factors"`
	// Missing names the factors turned on that had no reading.
	Missing []string `json:"missing"`
}

// Evaluate forecasts from the candles up to and including the last one. It reads
// nothing else, so nothing after that candle can affect it.
func Evaluate(config Config, interval string, klines []market.Kline, derivatives Derivatives) (Result, error) {
	if len(klines) < MinBars {
		return Result{}, ErrNotEnoughHistory
	}
	last := klines[len(klines)-1]
	atr := market.ATR(klines, 14)
	if atr <= 0 || last.Close <= 0 {
		return Result{}, ErrNothingToRead
	}

	adx := averageDirectionalIndex(klines, 14)
	regime := RegimeRanging
	if adx >= config.ADXThreshold {
		regime = RegimeTrending
	}

	values := candleFactorValues(klines, atr)
	derivativeValues(values, klines, atr, derivatives)

	result := Result{Close: last.Close, Regime: regime, ADX: adx, Horizon: config.Horizons[interval], Derivatives: derivatives, Missing: []string{}}

	var fullSum, fullWeight, candleSum, candleWeight, availableWeight, enabledWeight float64
	for _, name := range sortedFactors(config) {
		factor := config.Factors[name]
		if !factor.Enabled {
			continue
		}
		weight := factor.Ranging
		if regime == RegimeTrending {
			weight = factor.Trending
		}
		enabledWeight += math.Abs(weight)
		value, ok := values[name]
		if !ok {
			result.Missing = append(result.Missing, name)
			continue
		}
		availableWeight += math.Abs(weight)
		fullSum += weight * value
		fullWeight += math.Abs(weight)
		if !isDerivativeFactor(name) {
			candleSum += weight * value
			candleWeight += math.Abs(weight)
		} else {
			result.HasDerivatives = true
		}
		result.Factors = append(result.Factors, FactorValue{Name: name, Value: value, Weight: weight, Contribution: weight * value})
	}
	if fullWeight == 0 {
		return Result{}, ErrNothingToRead
	}
	result.ProbFull = logistic(config.Slope, fullSum/fullWeight)
	result.ProbCandles = result.ProbFull
	if candleWeight > 0 {
		result.ProbCandles = logistic(config.Slope, candleSum/candleWeight)
	}
	sort.SliceStable(result.Factors, func(i, j int) bool {
		return math.Abs(result.Factors[i].Contribution) > math.Abs(result.Factors[j].Contribution)
	})

	switch {
	case result.ProbFull >= 0.5+config.DirectionThreshold:
		result.Direction = DirectionUp
	case result.ProbFull <= 0.5-config.DirectionThreshold:
		result.Direction = DirectionDown
	default:
		result.Direction = DirectionNeutral
	}
	coverage := 1.0
	if enabledWeight > 0 {
		coverage = availableWeight / enabledWeight
	}
	result.Confidence = int(math.Round(math.Abs(2*result.ProbFull-1) * coverage * 100))
	result.ExpectedMovePct = atr / last.Close * math.Sqrt(float64(result.Horizon)) * 100
	result.Support, result.Resistance = supportResistance(klines)
	return result, nil
}

func logistic(slope, x float64) float64 {
	return 1 / (1 + math.Exp(-slope*x))
}

func isDerivativeFactor(name string) bool {
	for _, derivative := range DerivativeFactors {
		if derivative == name {
			return true
		}
	}
	return false
}

func clamp(value float64) float64 {
	return math.Max(-1, math.Min(1, value))
}

// candleFactorValues reads every factor that needs only candles. Each is from -1
// (points down) to 1 (points up).
func candleFactorValues(klines []market.Kline, atr float64) map[string]float64 {
	last := klines[len(klines)-1]
	values := map[string]float64{}

	// Trend: the distance between a fast and a slow average, in ATRs.
	values[FactorTrend] = math.Tanh((market.EMA(klines, 20) - market.EMA(klines, 50)) / atr / 2)

	// Momentum: the MACD line, in ATRs.
	values[FactorMomentum] = math.Tanh(market.MACD(klines) / atr)

	// RSI: above or below the midpoint.
	values[FactorRSI] = clamp((market.RSI(klines, 14) - 50) / 50)

	// Bollinger: where the close sits in the bands.
	if upper, middle, _ := market.Bollinger(klines, 20, 2); upper > middle {
		values[FactorBollinger] = clamp((last.Close - middle) / (upper - middle))
	}

	// Returns: moves over several spans, each scaled by what is normal for it.
	var returns float64
	spans := []int{1, 4, 12}
	for _, span := range spans {
		move := last.Close - klines[len(klines)-1-span].Close
		returns += math.Tanh(move / (atr * math.Sqrt(float64(span))))
	}
	values[FactorReturns] = returns / float64(len(spans))

	// Volume: who was the aggressor lately, counting more when volume is high.
	values[FactorVolume] = volumeFactor(klines)

	// Candle: the last candle's shape.
	values[FactorCandle] = candleFactor(klines)

	// Structure: higher highs and lows, or lower ones.
	values[FactorStructure] = structureFactor(klines)

	// Stochastic: where the close sits in the recent range.
	values[FactorStochastic] = stochasticFactor(klines, 14)

	// VWAP: distance from the rolling volume-weighted average price, in ATRs.
	values[FactorVWAP] = vwapFactor(klines, 24, atr)

	// Divergence: price making a new extreme that RSI does not confirm.
	values[FactorDivergence] = divergenceFactor(klines)
	return values
}

// derivativeValues adds the factors that need derivatives data, when it exists.
func derivativeValues(values map[string]float64, klines []market.Kline, atr float64, d Derivatives) {
	// Funding: when one side pays heavily, that side is crowded.
	if d.Funding != nil {
		values[FactorFunding] = -math.Tanh(*d.Funding / 0.0005)
	}
	// Open interest: new money behind the move confirms it; money leaving fades it.
	if d.OIChangePct != nil {
		move := klines[len(klines)-1].Close - klines[len(klines)-1-oiLookbackBars].Close
		values[FactorOpenInterest] = math.Tanh(*d.OIChangePct/5) * math.Tanh(move/(atr*math.Sqrt(oiLookbackBars)))
	}
	// Long/short ratio: a side unusually crowded for this coin tends to unwind.
	if d.LongShortZ != nil {
		values[FactorLongShort] = -math.Tanh(*d.LongShortZ / 2)
	}
}

func volumeFactor(klines []market.Kline) float64 {
	const recent, baseline = 3, 20
	n := len(klines)
	var buy, volume, average float64
	for _, k := range klines[n-recent:] {
		buy += k.TakerBuyBaseVolume
		volume += k.Volume
	}
	for _, k := range klines[n-baseline:] {
		average += k.Volume
	}
	if volume <= 0 || average <= 0 {
		return 0
	}
	imbalance := 2*buy/volume - 1
	relative := (volume / recent) / (average / baseline)
	return clamp(imbalance * 2 * math.Min(relative, 1.5) / 1.5)
}

func candleFactor(klines []market.Kline) float64 {
	last, previous := klines[len(klines)-1], klines[len(klines)-2]
	span := last.High - last.Low
	if span <= 0 {
		return 0
	}
	body := (last.Close - last.Open) / span
	lowerWick := (math.Min(last.Open, last.Close) - last.Low) / span
	upperWick := (last.High - math.Max(last.Open, last.Close)) / span
	value := 0.6*body + 0.4*(lowerWick-upperWick)
	// An engulfing candle reverses the one before it.
	if last.Close > last.Open && previous.Close < previous.Open && last.Close >= previous.Open && last.Open <= previous.Close {
		value += 0.3
	}
	if last.Close < last.Open && previous.Close > previous.Open && last.Close <= previous.Open && last.Open >= previous.Close {
		value -= 0.3
	}
	return clamp(value)
}

// swing finds turning points: a bar higher (or lower) than the bars either side.
func swing(klines []market.Kline, strength int, high bool) []int {
	var points []int
	for i := strength; i < len(klines)-strength; i++ {
		isPoint := true
		for j := i - strength; j <= i+strength && isPoint; j++ {
			if j == i {
				continue
			}
			if high && klines[j].High > klines[i].High {
				isPoint = false
			}
			if !high && klines[j].Low < klines[i].Low {
				isPoint = false
			}
		}
		if isPoint {
			points = append(points, i)
		}
	}
	return points
}

func sign(value float64) float64 {
	switch {
	case value > 0:
		return 1
	case value < 0:
		return -1
	}
	return 0
}

func structureFactor(klines []market.Kline) float64 {
	recent := klines[len(klines)-60:]
	highs, lows := swing(recent, 3, true), swing(recent, 3, false)
	if len(highs) < 2 || len(lows) < 2 {
		return 0
	}
	highMove := sign(recent[highs[len(highs)-1]].High - recent[highs[len(highs)-2]].High)
	lowMove := sign(recent[lows[len(lows)-1]].Low - recent[lows[len(lows)-2]].Low)
	return (highMove + lowMove) / 2
}

func stochasticFactor(klines []market.Kline, period int) float64 {
	recent := klines[len(klines)-period:]
	high, low := recent[0].High, recent[0].Low
	for _, k := range recent {
		high = math.Max(high, k.High)
		low = math.Min(low, k.Low)
	}
	if high <= low {
		return 0
	}
	return clamp(2*(recent[len(recent)-1].Close-low)/(high-low) - 1)
}

func vwapFactor(klines []market.Kline, period int, atr float64) float64 {
	vwap, ok := rollingVWAP(klines, period)
	if !ok {
		return 0
	}
	return math.Tanh((klines[len(klines)-1].Close - vwap) / atr / 2)
}

// rollingVWAP is the volume-weighted average price of the last period candles.
func rollingVWAP(klines []market.Kline, period int) (float64, bool) {
	var priced, volume float64
	for _, k := range klines[max(0, len(klines)-period):] {
		priced += (k.High + k.Low + k.Close) / 3 * k.Volume
		volume += k.Volume
	}
	if volume <= 0 {
		return 0, false
	}
	return priced / volume, true
}

// rsiSeries returns RSI for every bar, with Wilder smoothing.
func rsiSeries(klines []market.Kline, period int) []float64 {
	series := make([]float64, len(klines))
	if len(klines) <= period {
		return series
	}
	var gain, loss float64
	for i := 1; i <= period; i++ {
		change := klines[i].Close - klines[i-1].Close
		if change > 0 {
			gain += change
		} else {
			loss -= change
		}
	}
	gain /= float64(period)
	loss /= float64(period)
	rsi := func() float64 {
		if loss == 0 {
			return 100
		}
		return 100 - 100/(1+gain/loss)
	}
	series[period] = rsi()
	for i := period + 1; i < len(klines); i++ {
		change := klines[i].Close - klines[i-1].Close
		up, down := 0.0, 0.0
		if change > 0 {
			up = change
		} else {
			down = -change
		}
		gain = (gain*float64(period-1) + up) / float64(period)
		loss = (loss*float64(period-1) + down) / float64(period)
		series[i] = rsi()
	}
	return series
}

// divergenceFactor compares the last two swing points of price and RSI, fading
// as the signal ages over the 30 bars it can still matter.
func divergenceFactor(klines []market.Kline) float64 {
	const lookback, maxAge = 60, 30
	rsi := rsiSeries(klines, 14)
	offset := len(klines) - lookback
	recent := klines[offset:]
	var value float64
	if lows := swing(recent, 3, false); len(lows) >= 2 {
		a, b := lows[len(lows)-2], lows[len(lows)-1]
		if recent[b].Low < recent[a].Low && rsi[offset+b] > rsi[offset+a] {
			if age := len(recent) - 1 - b; age < maxAge {
				value += 1 - float64(age)/maxAge
			}
		}
	}
	if highs := swing(recent, 3, true); len(highs) >= 2 {
		a, b := highs[len(highs)-2], highs[len(highs)-1]
		if recent[b].High > recent[a].High && rsi[offset+b] < rsi[offset+a] {
			if age := len(recent) - 1 - b; age < maxAge {
				value -= 1 - float64(age)/maxAge
			}
		}
	}
	return clamp(value)
}

// averageDirectionalIndex measures how strongly the market trends, up or down.
func averageDirectionalIndex(klines []market.Kline, period int) float64 {
	n := len(klines)
	if n < 2*period+1 {
		return 0
	}
	var trSum, plusSum, minusSum float64
	dx := func() float64 {
		if trSum == 0 {
			return 0
		}
		plus, minus := 100*plusSum/trSum, 100*minusSum/trSum
		if plus+minus == 0 {
			return 0
		}
		return 100 * math.Abs(plus-minus) / (plus + minus)
	}
	var adx float64
	seeded := 0
	for i := 1; i < n; i++ {
		upMove := klines[i].High - klines[i-1].High
		downMove := klines[i-1].Low - klines[i].Low
		tr := math.Max(klines[i].High-klines[i].Low, math.Max(math.Abs(klines[i].High-klines[i-1].Close), math.Abs(klines[i].Low-klines[i-1].Close)))
		plus, minus := 0.0, 0.0
		if upMove > downMove && upMove > 0 {
			plus = upMove
		}
		if downMove > upMove && downMove > 0 {
			minus = downMove
		}
		if i <= period {
			trSum += tr
			plusSum += plus
			minusSum += minus
			if i == period {
				adx = dx()
				seeded = 1
			}
			continue
		}
		trSum = trSum - trSum/float64(period) + tr
		plusSum = plusSum - plusSum/float64(period) + plus
		minusSum = minusSum - minusSum/float64(period) + minus
		if seeded < period {
			adx += dx()
			seeded++
			if seeded == period {
				adx /= float64(period)
			}
			continue
		}
		adx = (adx*float64(period-1) + dx()) / float64(period)
	}
	return adx
}

// supportResistance returns the nearest swing levels below and above the close,
// falling back on the recent range's edges.
func supportResistance(klines []market.Kline) (support, resistance float64) {
	recent := klines[len(klines)-100:]
	close := recent[len(recent)-1].Close
	for _, i := range swing(recent, 3, false) {
		if low := recent[i].Low; low < close && low > support {
			support = low
		}
	}
	for _, i := range swing(recent, 3, true) {
		if high := recent[i].High; high > close && (resistance == 0 || high < resistance) {
			resistance = high
		}
	}
	if support == 0 || resistance == 0 {
		low, high := recent[0].Low, recent[0].High
		for _, k := range recent {
			low = math.Min(low, k.Low)
			high = math.Max(high, k.High)
		}
		if support == 0 {
			support = low
		}
		if resistance == 0 {
			resistance = high
		}
	}
	return support, resistance
}
