package domain

import "errors"

// BodyComposition represents body composition metrics from Apple Health.
// It's a pure domain entity: adapters map their own wire DTOs onto it rather
// than unmarshalling JSON straight into it.
type BodyComposition struct {
	BMI           float64
	FatPercentage float64 // %
	Weight        float64 // kg
	Timestamp     int64   // Unix timestamp (ms)
	AppleHealthID string  // UUID from Apple Health for idempotency

	// MeasurementDate is the calendar day (YYYY-MM-DD) that this measurement's
	// row is keyed on in storage — the aggregate's real identity, since
	// storage holds one row per day rather than one per Timestamp. It's
	// populated by the repository on read (FindUnsynced) and used for
	// identity matching (MarkSynced) instead of re-deriving it from Timestamp;
	// intake paths (Save) don't need to set it since the repository derives
	// it fresh from Timestamp at insert time.
	MeasurementDate string
}

// Validate checks if the measurement is valid.
func (m *BodyComposition) Validate() error {
	if m.BMI <= 0 || m.BMI > 100 {
		return errors.New("invalid BMI")
	}
	if m.FatPercentage < 0 || m.FatPercentage > 100 {
		return errors.New("invalid fat percentage")
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
	if m.Weight <= 0 && m.BMI <= 0 && m.FatPercentage <= 0 {
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
	return nil
}
