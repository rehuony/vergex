package okx

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type configTransport string

func (answer configTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Path != okxAccountConfigPath || r.Method != http.MethodGet {
		return nil, http.ErrUseLastResponse
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(answer)))}, nil
}

func TestPermissionsReadWhatOKXReports(t *testing.T) {
	read := func(config string) *Account {
		return &Account{httpClient: &http.Client{Transport: configTransport(`{"code":"0","msg":"","data":[` + config + `]}`)}}
	}
	got, err := read(`{"perm":"read_only,trade","ip":"203.0.113.7"}`).Permissions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Read || !got.Trade || got.Withdraw || !got.IPRestricted {
		t.Errorf("permissions = %+v", got)
	}
	got, err = read(`{"perm":"read_only,withdraw","ip":""}`).Permissions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Read || got.Trade || !got.Withdraw || got.IPRestricted {
		t.Errorf("permissions = %+v", got)
	}
	// An answer that says nothing about the key is not a key that may do nothing.
	got, err = read(`{"posMode":"long_short_mode"}`).Permissions(context.Background())
	if got != nil || err != nil {
		t.Errorf("permissions = %+v, err %v", got, err)
	}
}
