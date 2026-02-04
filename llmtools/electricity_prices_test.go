package llmtools

import (
	"strings"
	"testing"
	"time"
)

func TestFormatElectricityPrices(t *testing.T) {
	// Use a fixed location for consistent test results
	helsinki, _ := time.LoadLocation("Europe/Helsinki")

	tests := []struct {
		name         string
		prices       []PriceEntry
		wantContains []string
		wantEmpty    bool
	}{
		{
			name:      "empty prices",
			prices:    []PriceEntry{},
			wantEmpty: true,
		},
		{
			name:      "nil prices",
			prices:    nil,
			wantEmpty: true,
		},
		{
			name: "single price entry",
			prices: []PriceEntry{
				{Time: time.Date(2024, 1, 15, 10, 0, 0, 0, helsinki), Price: 5.50},
			},
			wantContains: []string{
				"Electricity Prices",
				"c/kWh",
				"Average: 5.50",
				"Min: 5.50",
				"Max: 5.50",
				"10:00", // Already in Helsinki timezone
				"5.50",
			},
		},
		{
			name: "multiple price entries",
			prices: []PriceEntry{
				{Time: time.Date(2024, 1, 15, 8, 0, 0, 0, helsinki), Price: 3.00},
				{Time: time.Date(2024, 1, 15, 9, 0, 0, 0, helsinki), Price: 5.00},
				{Time: time.Date(2024, 1, 15, 10, 0, 0, 0, helsinki), Price: 7.00},
			},
			wantContains: []string{
				"Average: 5.00",
				"Min: 3.00",
				"Max: 7.00",
				"Hourly prices:",
			},
		},
		{
			name: "negative prices",
			prices: []PriceEntry{
				{Time: time.Date(2024, 1, 15, 10, 0, 0, 0, helsinki), Price: -2.50},
				{Time: time.Date(2024, 1, 15, 11, 0, 0, 0, helsinki), Price: 1.50},
			},
			wantContains: []string{
				"Min: -2.50",
				"Max: 1.50",
				"-2.50",
			},
		},
		{
			name: "high prices",
			prices: []PriceEntry{
				{Time: time.Date(2024, 1, 15, 10, 0, 0, 0, helsinki), Price: 150.00},
				{Time: time.Date(2024, 1, 15, 11, 0, 0, 0, helsinki), Price: 200.00},
			},
			wantContains: []string{
				"Max: 200.00",
				"150.00",
				"200.00",
			},
		},
		{
			name: "prices with decimals",
			prices: []PriceEntry{
				{Time: time.Date(2024, 1, 15, 10, 0, 0, 0, helsinki), Price: 3.14159},
			},
			wantContains: []string{
				"3.14", // Should be formatted to 2 decimal places
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatElectricityPrices(tt.prices)

			if tt.wantEmpty {
				if !strings.Contains(result, "No electricity price data available") {
					t.Errorf("formatElectricityPrices() = %q, want empty message", result)
				}
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("formatElectricityPrices() = %q, want to contain %q", result, want)
				}
			}
		})
	}
}

func TestFormatElectricityPricesStatistics(t *testing.T) {
	helsinki, _ := time.LoadLocation("Europe/Helsinki")

	// Create a full day of prices to test statistics calculation
	prices := []PriceEntry{
		{Time: time.Date(2024, 1, 15, 0, 0, 0, 0, helsinki), Price: 2.00},
		{Time: time.Date(2024, 1, 15, 1, 0, 0, 0, helsinki), Price: 4.00},
		{Time: time.Date(2024, 1, 15, 2, 0, 0, 0, helsinki), Price: 6.00},
		{Time: time.Date(2024, 1, 15, 3, 0, 0, 0, helsinki), Price: 8.00},
	}

	result := formatElectricityPrices(prices)

	// Average should be (2+4+6+8)/4 = 5.00
	if !strings.Contains(result, "Average: 5.00") {
		t.Errorf("Expected average of 5.00, got: %s", result)
	}

	// Min should be 2.00
	if !strings.Contains(result, "Min: 2.00") {
		t.Errorf("Expected min of 2.00, got: %s", result)
	}

	// Max should be 8.00
	if !strings.Contains(result, "Max: 8.00") {
		t.Errorf("Expected max of 8.00, got: %s", result)
	}
}

func TestFormatElectricityPricesTimeFormat(t *testing.T) {
	helsinki, _ := time.LoadLocation("Europe/Helsinki")

	prices := []PriceEntry{
		{Time: time.Date(2024, 1, 15, 9, 0, 0, 0, helsinki), Price: 5.00},
	}

	result := formatElectricityPrices(prices)

	// Time should be formatted as HH:MM in Helsinki timezone
	if !strings.Contains(result, "09:00") {
		t.Errorf("Expected time format 09:00, got: %s", result)
	}
}

func TestFormatElectricityPricesUTCConversion(t *testing.T) {
	// Test that UTC times are properly converted to Helsinki time
	utc := time.UTC

	prices := []PriceEntry{
		// 10:00 UTC in January should be 12:00 Helsinki (UTC+2)
		{Time: time.Date(2024, 1, 15, 10, 0, 0, 0, utc), Price: 5.00},
	}

	result := formatElectricityPrices(prices)

	// Should show Helsinki time (12:00)
	if !strings.Contains(result, "12:00") {
		t.Errorf("Expected Helsinki time 12:00, got: %s", result)
	}
}
