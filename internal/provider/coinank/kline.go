package coinank

type KlineResult struct {
	StartTime int64   `json:"startTime"`
	EndTime   int64   `json:"endTime"`
	Open      float64 `json:"open"`
	Close     float64 `json:"close"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Volume    float64 `json:"volume"`
	Quantity  float64 `json:"quantity"`
	Count     float64 `json:"count"`
}
