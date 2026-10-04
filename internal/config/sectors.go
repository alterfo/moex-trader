package config

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

func DefaultSectors() map[string]string {
	return map[string]string{
		"SBER": "banks",
		"VTBR": "banks",
		"T":    "banks",
		"LKOH": "oil_gas",
		"ROSN": "oil_gas",
		"TATN": "oil_gas",
		"NVTK": "oil_gas",
		"GAZP": "oil_gas",
		"GMKN": "metals",
		"PLZL": "metals",
		"CHMF": "metals",
		"RUAL": "metals",
	}
}

func ParseSectors(s string) (map[string]string, error) {
	out := map[string]string{}
	for _, part := range splitComma(s) {
		pair := strings.SplitN(part, "=", 2)
		if len(pair) != 2 || strings.TrimSpace(pair[0]) == "" || strings.TrimSpace(pair[1]) == "" {
			return nil, fmt.Errorf("invalid sectors entry %q: want ticker=sector", part)
		}
		out[strings.ToUpper(strings.TrimSpace(pair[0]))] = strings.TrimSpace(pair[1])
	}
	return out, nil
}

func ParseSectorCaps(s string) (map[string]decimal.Decimal, error) {
	out := map[string]decimal.Decimal{}
	for _, part := range splitComma(s) {
		pair := strings.SplitN(part, "=", 2)
		if len(pair) != 2 || strings.TrimSpace(pair[0]) == "" || strings.TrimSpace(pair[1]) == "" {
			return nil, fmt.Errorf("invalid sector cap entry %q: want sector=notional", part)
		}
		value, err := decimal.NewFromString(strings.TrimSpace(pair[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid sector cap for %q: %w", strings.TrimSpace(pair[0]), err)
		}
		if value.IsNegative() {
			return nil, fmt.Errorf("sector cap for %q must be non-negative", strings.TrimSpace(pair[0]))
		}
		out[strings.TrimSpace(pair[0])] = value
	}
	return out, nil
}
