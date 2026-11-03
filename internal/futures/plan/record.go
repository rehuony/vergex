package plan

import "time"

type Record struct {
	Plan    Plan      `json:"plan"`
	Status  string    `json:"status"`
	Updated time.Time `json:"updated"`
}
