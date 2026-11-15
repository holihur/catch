package auth

import (
	"net/http"
	"time"
)

type httpClient struct {
	HTTP *http.Client
}

func newHTTPClient() *httpClient {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = 50
	tr.IdleConnTimeout = 90 * time.Second
	return &httpClient{HTTP: &http.Client{Transport: tr, Timeout: 20 * time.Second}}
}
