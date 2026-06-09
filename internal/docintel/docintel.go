package docintel

import "net/http"

const (
	apiVersion = "2024-11-30"
	modelID    = "prebuilt-layout"
)

type Client struct {
	endpoint string
	http     *http.Client
}

func NewClient(docintelURL, docintelKey string) *Client {
	return &Client{
		endpoint: docintelURL,
		http: &http.Client{
			Transport: &httpTransport{
				RoundTripper: http.DefaultTransport,
				apiKey:       docintelKey,
			},
		},
	}
}
