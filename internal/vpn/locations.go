package vpn

import (
	"strings"
)

type VPNLocation struct {
	Country     string `json:"country"`
	City        string `json:"city"`
	Alias       string `json:"alias"`
	DisplayName string `json:"displayName"`
	Region      string `json:"region"`
	Flag        string `json:"flag"`
	Recommended bool   `json:"recommended"`
}

// Fallback Static Location Catalog
var FallbackLocations = []VPNLocation{
	{Country: "Vietnam", City: "Vietnam", Alias: "vietnam", DisplayName: "Vietnam", Region: "Asia Pacific", Flag: "🇻🇳", Recommended: true},
	{Country: "Singapore", City: "Singapore", Alias: "singapore", DisplayName: "Singapore", Region: "Asia Pacific", Flag: "🇸🇬", Recommended: true},
	{Country: "Malaysia", City: "Malaysia", Alias: "malaysia", DisplayName: "Malaysia", Region: "Asia Pacific", Flag: "🇲🇾", Recommended: false},
	{Country: "Thailand", City: "Thailand", Alias: "thailand", DisplayName: "Thailand", Region: "Asia Pacific", Flag: "🇹🇭", Recommended: false},
	{Country: "Hong Kong", City: "Hong Kong", Alias: "hongkong", DisplayName: "Hong Kong", Region: "Asia Pacific", Flag: "🇭🇰", Recommended: true},
	{Country: "Japan", City: "Tokyo", Alias: "japan-tokyo", DisplayName: "Japan - Tokyo", Region: "Asia Pacific", Flag: "🇯🇵", Recommended: true},
	{Country: "South Korea", City: "Seoul", Alias: "southkorea", DisplayName: "South Korea", Region: "Asia Pacific", Flag: "🇰🇷", Recommended: false},
	{Country: "Australia", City: "Sydney", Alias: "australia", DisplayName: "Australia - Sydney", Region: "Asia Pacific", Flag: "🇦🇺", Recommended: true},
	{Country: "New Zealand", City: "Auckland", Alias: "newzealand", DisplayName: "New Zealand", Region: "Asia Pacific", Flag: "🇳🇿", Recommended: false},
	{Country: "United States", City: "New York", Alias: "usa-new-york", DisplayName: "United States - New York", Region: "Americas", Flag: "🇺🇸", Recommended: true},
	{Country: "United States", City: "Los Angeles", Alias: "usa-los-angeles", DisplayName: "United States - Los Angeles", Region: "Americas", Flag: "🇺🇸", Recommended: true},
	{Country: "Canada", City: "Toronto", Alias: "canada", DisplayName: "Canada - Toronto", Region: "Americas", Flag: "🇨🇦", Recommended: true},
	{Country: "Mexico", City: "Mexico City", Alias: "mexico", DisplayName: "Mexico", Region: "Americas", Flag: "🇲🇽", Recommended: false},
	{Country: "Brazil", City: "Sao Paulo", Alias: "brazil", DisplayName: "Brazil", Region: "Americas", Flag: "🇧🇷", Recommended: false},
	{Country: "Chile", City: "Santiago", Alias: "chile", DisplayName: "Chile", Region: "Americas", Flag: "🇨🇱", Recommended: false},
	{Country: "Argentina", City: "Buenos Aires", Alias: "argentina", DisplayName: "Argentina", Region: "Americas", Flag: "🇦🇷", Recommended: false},
	{Country: "United Kingdom", City: "London", Alias: "uk-london", DisplayName: "United Kingdom - London", Region: "Europe", Flag: "🇬🇧", Recommended: true},
	{Country: "Germany", City: "Frankfurt", Alias: "germany-frankfurt", DisplayName: "Germany - Frankfurt", Region: "Europe", Flag: "🇩🇪", Recommended: true},
	{Country: "France", City: "Paris", Alias: "france", DisplayName: "France - Paris", Region: "Europe", Flag: "🇫🇷", Recommended: true},
	{Country: "Netherlands", City: "Amsterdam", Alias: "netherlands", DisplayName: "Netherlands", Region: "Europe", Flag: "🇳🇱", Recommended: true},
}

// Region and Flag mapper helper
var RegionFlagMap = map[string]struct{ Region, Flag string }{
	"vietnam":        {"Asia Pacific", "🇻🇳"},
	"singapore":      {"Asia Pacific", "🇸🇬"},
	"malaysia":       {"Asia Pacific", "🇲🇾"},
	"thailand":       {"Asia Pacific", "🇹🇭"},
	"hong kong":      {"Asia Pacific", "🇭🇰"},
	"hongkong":       {"Asia Pacific", "🇭🇰"},
	"japan":          {"Asia Pacific", "🇯🇵"},
	"south korea":    {"Asia Pacific", "🇰🇷"},
	"southkorea":     {"Asia Pacific", "🇰🇷"},
	"australia":      {"Asia Pacific", "🇦🇺"},
	"new zealand":    {"Asia Pacific", "🇳🇿"},
	"newzealand":     {"Asia Pacific", "🇳🇿"},
	"united states":  {"Americas", "🇺🇸"},
	"usa":            {"Americas", "🇺🇸"},
	"canada":         {"Americas", "🇨🇦"},
	"mexico":         {"Americas", "🇲🇽"},
	"brazil":         {"Americas", "🇧🇷"},
	"chile":          {"Americas", "🇨🇱"},
	"argentina":      {"Americas", "🇦🇷"},
	"united kingdom": {"Europe", "🇬🇧"},
	"uk":             {"Europe", "🇬🇧"},
	"germany":        {"Europe", "🇩🇪"},
	"france":         {"Europe", "🇫🇷"},
	"netherlands":    {"Europe", "🇳🇱"},
}

// ParseExpressVpnLocations parses the stdout from `expressvpn list all`
func ParseExpressVpnLocations(output string) []VPNLocation {
	lines := strings.Split(output, "\n")
	var locations []VPNLocation

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-") || strings.HasPrefix(strings.ToLower(line), "alias") {
			continue
		}

		// ExpressVPN CLI format typically:
		// alias   displayName   [Y/N or Recommended]
		// e.g.:
		// vietnam  Vietnam       Y
		// or:
		// usny    USA - New York
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		alias := fields[0]
		
		// Find if there is a recommendation marker at the end
		recommended := false
		lastIndex := len(fields) - 1
		lastVal := strings.ToUpper(fields[lastIndex])
		if lastVal == "Y" || lastVal == "YES" || lastVal == "TRUE" {
			recommended = true
			fields = fields[:lastIndex] // strip recommendation marker
		}

		displayName := strings.Join(fields[1:], " ")

		// Determine country, city, region, flag
		country := displayName
		city := displayName
		if strings.Contains(displayName, "-") {
			parts := strings.Split(displayName, "-")
			country = strings.TrimSpace(parts[0])
			city = strings.TrimSpace(parts[1])
		}

		// Lookup region and flag
		region := "Other"
		flag := "🌐"
		lowerCountry := strings.ToLower(country)
		lowerAlias := strings.ToLower(alias)

		for key, mapped := range RegionFlagMap {
			if strings.Contains(lowerCountry, key) || strings.Contains(lowerAlias, key) {
				region = mapped.Region
				flag = mapped.Flag
				break
			}
		}

		locations = append(locations, VPNLocation{
			Country:     country,
			City:        city,
			Alias:       alias,
			DisplayName: displayName,
			Region:      region,
			Flag:        flag,
			Recommended: recommended,
		})
	}

	if len(locations) == 0 {
		return FallbackLocations
	}
	return locations
}
