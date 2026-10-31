package forecast

import (
	"encoding/json"
	"math"
	"sort"
)

// MinCalibrationSamples is how many non-overlapping verified forecasts a version
// and interval need before their scores are shown as probabilities.
const MinCalibrationSamples = 200

// Knot is one point of a calibration curve.
type Knot struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// CalibrationPoint is one verified forecast: what it said and what happened.
type CalibrationPoint struct {
	Prob float64
	Up   bool
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
