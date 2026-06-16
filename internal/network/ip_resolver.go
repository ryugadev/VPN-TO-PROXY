package network

import (
	"context"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

var IPProviders = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://icanhazip.com",
}

// ResolvePublicIP queries multiple external providers in order to find the current public exit IP.
func ResolvePublicIP(ctx context.Context) (string, error) {
	var lastErr error
	for _, provider := range IPProviders {
		ip, err := queryProvider(ctx, provider)
		if err == nil {
			return ip, nil
		}
		lastErr = err
	}
	return "", lastErr
}

func queryProvider(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	ip := strings.TrimSpace(string(body))
	return ip, nil
}
