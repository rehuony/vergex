package forecast

import (
	"encoding/json"
	"math"
	"sort"
)

const (
	// MinCalibrationMoments is how many separate moments a strategy and interval need
	// before their scores can be shown as probabilities. Coins move together, so a
	// hundred coins over one horizon are far from a hundred independent answers.
	MinCalibrationMoments = 200
	// calibrationHoldOut is the newest share of the moments a trial curve is judged on.
	calibrationHoldOut = 0.3
	// calibrationEvidence is how many standard errors a trial curve must beat the base
	// rate by, about one chance in twenty of passing on noise.
	calibrationEvidence = 1.65
)

// Knot is one point of a calibration curve.
type Knot struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// CalibrationPoint is one verified forecast: what it said, what happened and which
// moment it belongs to. A moment is one horizon's stretch of time.
type CalibrationPoint struct {
	Prob   float64
	Up     bool
	Moment int64
}

// countMoments says how many separate moments the points come from.
func countMoments(points []CalibrationPoint) int {
	moments := map[int64]bool{}
	for _, point := range points {
		moments[point.Moment] = true
	}
	return len(moments)
}

// EarnCalibration returns a curve only if the points have earned one: they span enough
// moments, and a curve fitted on the older ones clearly predicts the newest better than
// simply knowing how often price rose. Each moment counts once, however many coins it
// holds. Scores with no relation to outcomes rarely pass, and so stay scores.
func EarnCalibration(points []CalibrationPoint) ([]Knot, bool) {
	if countMoments(points) < MinCalibrationMoments {
		return nil, false
	}
	ordered := append([]CalibrationPoint{}, points...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Moment < ordered[j].Moment })
	moments := []int64{}
	for _, point := range ordered {
		if len(moments) == 0 || moments[len(moments)-1] != point.Moment {
			moments = append(moments, point.Moment)
		}
	}
	firstHeld := moments[int(float64(len(moments))*(1-calibrationHoldOut))]
	split := sort.Search(len(ordered), func(i int) bool { return ordered[i].Moment >= firstHeld })
	older, newer := ordered[:split], ordered[split:]

	curve := FitCalibration(older)
	var rose float64
	for _, point := range older {
		if point.Up {
			rose++
		}
	}
	base := rose / float64(len(older))

	// How much closer the curve came than the base rate, moment by moment.
	type gain struct{ sum, count float64 }
	gains := map[int64]*gain{}
	for _, point := range newer {
		outcome := 0.0
		if point.Up {
			outcome = 1
		}
		said := ApplyCalibration(curve, point.Prob)
		if gains[point.Moment] == nil {
			gains[point.Moment] = &gain{}
		}
		gains[point.Moment].sum += (base-outcome)*(base-outcome) - (said-outcome)*(said-outcome)
		gains[point.Moment].count++
	}
	var total, squares float64
	for _, moment := range gains {
		total += moment.sum / moment.count
	}
	n := float64(len(gains))
	mean := total / n
	for _, moment := range gains {
		difference := moment.sum/moment.count - mean
		squares += difference * difference
	}
	if mean <= 0 || mean < calibrationEvidence*math.Sqrt(squares/n/n) {
		return nil, false
	}
	return FitCalibration(points), true
}

// FitCalibration fits a curve that only ever rises (isotonic regression), so a
// higher score never maps to a lower probability.
func FitCalibration(points []CalibrationPoint) []Knot {
	sorted := append([]CalibrationPoint{}, points...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Prob < sorted[j].Prob })

	type block struct {
		sumX, sumY, count float64
	}
	var blocks []block
	for _, point := range sorted {
		y := 0.0
		if point.Up {
			y = 1
		}
		blocks = append(blocks, block{sumX: point.Prob, sumY: y, count: 1})
		for len(blocks) > 1 {
			a, b := blocks[len(blocks)-2], blocks[len(blocks)-1]
			if a.sumY/a.count < b.sumY/b.count {
				break
			}
			blocks = blocks[:len(blocks)-1]
			blocks[len(blocks)-1] = block{sumX: a.sumX + b.sumX, sumY: a.sumY + b.sumY, count: a.count + b.count}
		}
	}
	knots := make([]Knot, 0, len(blocks))
	for _, b := range blocks {
		knots = append(knots, Knot{X: b.sumX / b.count, Y: math.Max(0.01, math.Min(0.99, b.sumY/b.count))})
	}
	return knots
}

// ApplyCalibration reads a raw probability off the curve.
func ApplyCalibration(knots []Knot, prob float64) float64 {
	if len(knots) == 0 {
		return prob
	}
	if prob <= knots[0].X {
		return knots[0].Y
	}
	last := knots[len(knots)-1]
	if prob >= last.X {
		return last.Y
	}
	i := sort.Search(len(knots), func(i int) bool { return knots[i].X >= prob })
	a, b := knots[i-1], knots[i]
	if b.X == a.X {
		return b.Y
	}
	return a.Y + (b.Y-a.Y)*(prob-a.X)/(b.X-a.X)
}

func encodeKnots(knots []Knot) string {
	body, _ := json.Marshal(knots)
	return string(body)
}

func decodeKnots(text string) []Knot {
	var knots []Knot
	_ = json.Unmarshal([]byte(text), &knots)
	return knots
}
