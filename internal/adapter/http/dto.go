package http

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// MeasurementType represents the type of measurement being sent.
type MeasurementType string

const (
	MeasurementWeight     MeasurementType = "weight"
	MeasurementBMI        MeasurementType = "bmi"
	MeasurementFatPercent MeasurementType = "fat"
)

// FlexibleFloat64 accepts JSON numbers or numeric strings.
type FlexibleFloat64 float64

func (f *FlexibleFloat64) UnmarshalJSON(b []byte) error {
	// try as number
	var num float64
	if err := json.Unmarshal(b, &num); err == nil {
		*f = FlexibleFloat64(num)
		return nil
	}
	// try as string
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		if s == "" {
			*f = 0
			return nil
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("cannot parse numeric string %q: %w", s, err)
		}
		*f = FlexibleFloat64(n)
		return nil
	}
	return fmt.Errorf("unsupported json for FlexibleFloat64: %s", string(b))
}

// Float64 returns the value as float64.
func (f FlexibleFloat64) Float64() float64 { return float64(f) }

// FullBodyCompositionRequest is the wire shape for a full body-composition
// payload from the Shortcut (all metrics in one request).
type FullBodyCompositionRequest struct {
	BMI           float64 `json:"bmi"`
	FatPercentage float64 `json:"fat_percentage"`
	Weight        float64 `json:"weight"`
	Timestamp     int64   `json:"timestamp"`
	AppleHealthID string  `json:"apple_health_id"`
}

// SingleMeasurementRequest represents a single measurement value from Apple Shortcut.
type SingleMeasurementRequest struct {
	Type      MeasurementType `json:"type"`
	Value     FlexibleFloat64 `json:"value"`
	Timestamp int64           `json:"timestamp,omitempty"`
	ID        string          `json:"id,omitempty"`
}

// RawWeightRequest represents a raw weight-only request (e.g., {"weight":"80.35"}).
type RawWeightRequest struct {
	Weight FlexibleFloat64 `json:"weight"`
}

// RawBMIRequest represents a raw BMI-only request (e.g., {"bmi":"24.5"}).
type RawBMIRequest struct {
	BMI FlexibleFloat64 `json:"bmi"`
}

// RawFatPercentageRequest represents a raw fat percentage-only request.
type RawFatPercentageRequest struct {
	FatPercentage FlexibleFloat64 `json:"fat"` // accept both "fat" and "fat_percentage" depending on sender
	// also accept fat_percentage field
	FatPercentageAlt FlexibleFloat64 `json:"fat_percentage"`
}

// MeasurementResponse represents the API response.
type MeasurementResponse struct {
	Synced      bool   `json:"synced"`
	Measurement string `json:"measurement,omitempty"`
	Message     string `json:"message,omitempty"`
}

// HealthResponse represents the health check response.
type HealthResponse struct {
	Status string `json:"status"`
}
