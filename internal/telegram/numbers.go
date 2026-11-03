package telegram

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// amount is money in USDT: two decimals under a thousand, then one with K or M.
func amount(value float64) string {
	size := math.Abs(value)
	switch {
	case size >= 1e6:
		return strconv.FormatFloat(value/1e6, 'f', 1, 64) + "M"
	case size >= 1e3:
		return strconv.FormatFloat(value/1e3, 'f', 1, 64) + "K"
	}
	return strconv.FormatFloat(value, 'f', 2, 64)
}

// change is an amount with its sign, as a gain or a loss.
func change(value float64) string {
	if value >= 0 {
		return "+" + amount(value)
	}
	return amount(value)
}

// dollars is what a model cost, as its provider bills it.
func dollars(value float64) string { return "$" + strconv.FormatFloat(value, 'f', 2, 64) }

// exact is a price or a quantity as the exchange gives it, never with an exponent, its
// thousands separated.
func exact(value float64) string { return grouped(strconv.FormatFloat(value, 'f', -1, 64)) }

// exacts gives a column of quantities the decimals of its longest one, so they line up.
func exacts(values []float64) []string {
	decimals := 0
	for _, value := range values {
		if _, fraction, ok := strings.Cut(strconv.FormatFloat(value, 'f', -1, 64), "."); ok {
			decimals = max(decimals, len(fraction))
		}
	}
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = grouped(strconv.FormatFloat(value, 'f', decimals, 64))
	}
	return out
}

// grouped separates the thousands of a number's whole part.
func grouped(number string) string {
	sign, whole, fraction := "", number, ""
	if strings.HasPrefix(whole, "-") {
		sign, whole = "-", whole[1:]
	}
	if i := strings.IndexByte(whole, '.'); i >= 0 {
		whole, fraction = whole[:i], whole[i:]
	}
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "," + whole[i:]
	}
	return sign + whole + fraction
}

// moment is a time as every message writes it.
func moment(t time.Time) string { return t.UTC().Format("01-02 15:04") + " UTC" }
