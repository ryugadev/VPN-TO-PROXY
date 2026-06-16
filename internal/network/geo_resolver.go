package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type GeoIPInfo struct {
	Country     string `json:"country"`
	CountryCode string `json:"countryCode"`
	RegionName  string `json:"regionName"`
	City        string `json:"city"`
	ISP         string `json:"isp"`
	AS          string `json:"as"`
	Timezone    string `json:"timezone"`
}

// Fallback mappings for local/mock testing IPs
var mockGeos = map[string]GeoIPInfo{
	"103.10.10.10": {
		Country:     "Vietnam",
		CountryCode: "VN",
		RegionName:  "Ho Chi Minh City",
		City:        "Ho Chi Minh City",
		ISP:         "Viettel Group",
		AS:          "AS7552",
		Timezone:    "Asia/Ho_Chi_Minh",
	},
	"45.90.90.10": {
		Country:     "Singapore",
		CountryCode: "SG",
		RegionName:  "Singapore",
		City:        "Singapore",
		ISP:         "M247",
		AS:          "AS9009",
		Timezone:    "Asia/Singapore",
	},
	"150.95.10.10": {
		Country:     "Japan",
		CountryCode: "JP",
		RegionName:  "Tokyo",
		City:        "Tokyo",
		ISP:         "GMO Internet",
		AS:          "AS3791",
		Timezone:    "Asia/Tokyo",
	},
	"198.51.100.10": {
		Country:     "United States",
		CountryCode: "US",
		RegionName:  "New York",
		City:        "New York",
		ISP:         "DigitalOcean",
		AS:          "AS14061",
		Timezone:    "America/New_York",
	},
	"10.200.0.2": {
		Country:     "Vietnam",
		CountryCode: "VN",
		RegionName:  "Hanoi",
		City:        "Hanoi",
		ISP:         "Viettel",
		AS:          "AS7552",
		Timezone:    "Asia/Ho_Chi_Minh",
	},
}

// ResolveGeoIP queries geolocation sequentially from ipinfo.io, ipwho.is, and ip-api.com
func ResolveGeoIP(ctx context.Context, ip string) (*GeoIPInfo, error) {
	ip = strings.TrimSpace(ip)

	// Check if this is a known mock IP
	if info, ok := mockGeos[ip]; ok {
		return &info, nil
	}

	// Filter out local IPs
	if ip == "" || strings.HasPrefix(ip, "127.") || strings.HasPrefix(ip, "10.") || strings.HasPrefix(ip, "192.168.") || ip == "0.0.0.0" {
		return &GeoIPInfo{
			Country:     "Local/Private Network",
			CountryCode: "LCL",
			RegionName:  "Private",
			City:        "Private",
			ISP:         "IANA",
			AS:          "AS0",
			Timezone:    "UTC",
		}, nil
	}

	// Fallback Chain
	var lastErr error
	var info *GeoIPInfo

	// 1. Try ipinfo.io
	info, lastErr = resolveIpInfo(ctx, ip)
	if lastErr == nil && info != nil {
		return info, nil
	}

	// 2. Try ipwho.is
	info, lastErr = resolveIpWhoIs(ctx, ip)
	if lastErr == nil && info != nil {
		return info, nil
	}

	// 3. Try ip-api.com
	info, lastErr = resolveIpApi(ctx, ip)
	if lastErr == nil && info != nil {
		return info, nil
	}

	return nil, fmt.Errorf("failed to resolve GeoIP details: %v", lastErr)
}

func resolveIpInfo(ctx context.Context, ip string) (*GeoIPInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://ipinfo.io/"+ip+"/json", nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ipinfo.io returned status %d", resp.StatusCode)
	}

	var raw struct {
		IP       string `json:"ip"`
		Country  string `json:"country"`
		Region   string `json:"region"`
		City     string `json:"city"`
		Org      string `json:"org"`
		Timezone string `json:"timezone"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	asn := "Unknown"
	isp := "Unknown"
	if raw.Org != "" {
		parts := strings.SplitN(raw.Org, " ", 2)
		if len(parts) > 0 {
			asn = parts[0]
		}
		if len(parts) > 1 {
			isp = parts[1]
		}
	}

	return &GeoIPInfo{
		Country:     raw.Country, // Note: ipinfo returns ISO code (e.g. US), we can map or use it
		CountryCode: raw.Country,
		RegionName:  raw.Region,
		City:        raw.City,
		ISP:         isp,
		AS:          asn,
		Timezone:    raw.Timezone,
	}, nil
}

func resolveIpWhoIs(ctx context.Context, ip string) (*GeoIPInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://ipwho.is/"+ip, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ipwho.is returned status %d", resp.StatusCode)
	}

	var raw struct {
		Success     bool   `json:"success"`
		Country     string `json:"country"`
		CountryCode string `json:"country_code"`
		Region      string `json:"region"`
		City        string `json:"city"`
		Connection  struct {
			ASN interface{} `json:"asn"`
			ISP string      `json:"isp"`
		} `json:"connection"`
		Timezone struct {
			ID string `json:"id"`
		} `json:"timezone"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	if !raw.Success {
		return nil, fmt.Errorf("ipwho.is resolution failed for IP %s", ip)
	}

	asnStr := "Unknown"
	switch v := raw.Connection.ASN.(type) {
	case string:
		asnStr = v
	case float64:
		asnStr = "AS" + strconv.Itoa(int(v))
	}

	return &GeoIPInfo{
		Country:     raw.Country,
		CountryCode: raw.CountryCode,
		RegionName:  raw.Region,
		City:        raw.City,
		ISP:         raw.Connection.ISP,
		AS:          asnStr,
		Timezone:    raw.Timezone.ID,
	}, nil
}

func resolveIpApi(ctx context.Context, ip string) (*GeoIPInfo, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "http://ip-api.com/json/"+ip, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ip-api.com returned status %d", resp.StatusCode)
	}

	var raw struct {
		Status      string `json:"status"`
		Country     string `json:"country"`
		CountryCode string `json:"countryCode"`
		RegionName  string `json:"regionName"`
		City        string `json:"city"`
		ISP         string `json:"isp"`
		AS          string `json:"as"`
		Timezone    string `json:"timezone"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	if raw.Status != "success" {
		return nil, fmt.Errorf("ip-api.com resolution failed for IP %s", ip)
	}

	asnStr := "Unknown"
	if raw.AS != "" {
		parts := strings.Split(raw.AS, " ")
		if len(parts) > 0 {
			asnStr = parts[0]
		}
	}

	return &GeoIPInfo{
		Country:     raw.Country,
		CountryCode: raw.CountryCode,
		RegionName:  raw.RegionName,
		City:        raw.City,
		ISP:         raw.ISP,
		AS:          asnStr,
		Timezone:    raw.Timezone,
	}, nil
}
