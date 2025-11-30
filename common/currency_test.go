package common

import (
	"testing"
)

func TestQuotaToUSD(t *testing.T) {
	tests := []struct {
		name     string
		quota    int
		expected float64
	}{
		{
			name:     "Zero quota",
			quota:    0,
			expected: 0.0,
		},
		{
			name:     "One dollar",
			quota:    500000,
			expected: 1.0,
		},
		{
			name:     "Negative quota",
			quota:    -500000,
			expected: -1.0,
		},
		{
			name:     "Large value",
			quota:    50000000, // $100
			expected: 100.0,
		},
		{
			name:     "Fractional dollar",
			quota:    250000, // $0.50
			expected: 0.5,
		},
		{
			name:     "Small fractional value",
			quota:    5000, // $0.01
			expected: 0.01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := QuotaToUSD(tt.quota)
			if result != tt.expected {
				t.Errorf("QuotaToUSD(%d) = %f; want %f", tt.quota, result, tt.expected)
			}
		})
	}
}

func TestFormatUSD(t *testing.T) {
	tests := []struct {
		name     string
		quota    int
		expected string
	}{
		{
			name:     "Zero quota",
			quota:    0,
			expected: "$0.00",
		},
		{
			name:     "One dollar",
			quota:    500000,
			expected: "$1.00",
		},
		{
			name:     "Negative quota",
			quota:    -500000,
			expected: "$-1.00",
		},
		{
			name:     "Large value",
			quota:    50000000, // $100
			expected: "$100.00",
		},
		{
			name:     "Fractional dollar",
			quota:    250000, // $0.50
			expected: "$0.50",
		},
		{
			name:     "Small fractional value",
			quota:    5000, // $0.01
			expected: "$0.01",
		},
		{
			name:     "Very large value",
			quota:    500000000, // $1000
			expected: "$1000.00",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatUSD(tt.quota)
			if result != tt.expected {
				t.Errorf("FormatUSD(%d) = %s; want %s", tt.quota, result, tt.expected)
			}
		})
	}
}
