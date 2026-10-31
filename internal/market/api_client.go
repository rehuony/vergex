package market

import (
	"net/http"
	"time"
)

type APIClient struct {
	client *http.Client
}

func NewAPIClient() *APIClient {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	return &APIClient{
		client: client,
	}
}
