package domain

import "errors"

// BodyComposition represents body composition metrics from Apple Health.
type BodyComposition struct {
	BMI              float64 `json:"bmi"`
	FatPercentage    float64 `json:"fat_percentage"`    // %
	LeanBodyMass     float64 `json:"lean_body_mass"`    // kg
	Weight           float64 `json:"weight"`            // kg
	Timestamp        int64   `json:"timestamp"`         // Unix timestamp (ms)
	AppleHealthID    string  `json:"apple_health_id"`   // UUID from Apple Health for idempotency
}

// Validate checks if the measurement is valid.
func (m *BodyComposition) Validate() error {
	if m.BMI <= 0 || m.BMI > 100 {
		return errors.New("invalid BMI")
	}
	if m.FatPercentage < 0 || m.FatPercentage > 100 {
		return errors.New("invalid fat percentage")
	}
	if m.LeanBodyMass <= 0 || m.LeanBodyMass > 500 {
		return errors.New("invalid lean body mass")
	}
	if m.Weight <= 0 || m.Weight > 500 {
		return errors.New("invalid weight")
	}
	if m.Timestamp <= 0 {
		return errors.New("invalid timestamp")
	}
	if m.AppleHealthID == "" {
		return errors.New("apple health ID is required")
	}
	return nil
}

// ValidatePartial checks if a partial measurement is valid (at least one metric must be present, along with ID and timestamp).
func (m *BodyComposition) ValidatePartial() error {
	if m.AppleHealthID == "" {
		return errors.New("apple health ID is required")
	}
	if m.Timestamp <= 0 {
		return errors.New("invalid timestamp")
	}
	if m.Weight <= 0 && m.BMI <= 0 && m.LeanBodyMass <= 0 && m.FatPercentage <= 0 {
		return errors.New("at least one body composition metric must be provided")
	}
	// Validate non-zero fields
	if m.Weight < 0 || m.Weight > 500 {
		return errors.New("invalid weight")
	}
	if m.BMI < 0 || m.BMI > 100 {
		return errors.New("invalid BMI")
	}
	if m.FatPercentage < 0 || m.FatPercentage > 100 {
		return errors.New("invalid fat percentage")
	}
	if m.LeanBodyMass < 0 || m.LeanBodyMass > 500 {
		return errors.New("invalid lean body mass")
	}
	return nil
}
