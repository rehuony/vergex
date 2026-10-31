package coinank

import (
	"errors"
)

// CoinankResponse coinank openapi common response
type CoinankResponse[T any] struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Data    T      `json:"data"`
}

// PageData coinank openapi pageData in response
type PageData[T any] struct {
	List       []T `json:"list"`
	Pagination struct {
		Current  int `json:"current"`
		Total    int `json:"total"`
		PageSize int `json:"pageSize"`
	} `json:"pagination"`
}

var HttpError error = errors.New("http client error")
