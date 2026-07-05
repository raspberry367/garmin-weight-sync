package garmin

import (
	"bytes"
	"fmt"
	"time"

	"github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/rsb/garmin-weight-sync/internal/domain"
)

const (
	// garminProduct/garminSerialNumber are cosmetic device identifiers
	// Garmin's backend expects on the FileId message (mechanism doc §5.1).
	garminProduct      = 2429
	garminSerialNumber = 1234

	defaultActivityClass = typedef.ActivityClass(90)

	// UserProfile's Gender/Age/Height are just protocol filler alongside the
	// weight measurement — Garmin files the WeightScale message under the
	// account that's already authenticated, not by these fields. Not worth
	// exposing as config.
	defaultGender = typedef.GenderMale
)

// encodeWeightFIT turns a BodyComposition measurement into a binary FIT
// File.Weight payload: FileId + UserProfile + WeightScale messages, in that
// order (mechanism doc §5.1). The dead DeviceInfo message from the reference
// implementation is intentionally omitted — it's built but never written
// there either.
func encodeWeightFIT(m *domain.BodyComposition) ([]byte, error) {
	ts := time.UnixMilli(m.Timestamp).UTC()

	fileID := mesgdef.NewFileId(nil).
		SetType(typedef.FileWeight).
		SetManufacturer(typedef.ManufacturerGarmin).
		SetProduct(garminProduct).
		SetSerialNumber(garminSerialNumber).
		SetTimeCreated(ts)

	up := mesgdef.NewUserProfile(nil).
		SetGender(defaultGender).
		SetActivityClass(defaultActivityClass).
		SetMessageIndex(0).
		SetLocalId(0)
	if m.Weight > 0 {
		up.SetWeightScaled(m.Weight)
	}

	ws := mesgdef.NewWeightScale(nil).
		SetTimestamp(ts).
		SetUserProfileIndex(0)
	if m.Weight > 0 {
		ws.SetWeightScaled(m.Weight)
	}
	if m.FatPercentage > 0 {
		ws.SetPercentFatScaled(m.FatPercentage)
	}
	if m.BMI > 0 {
		ws.SetBmiScaled(m.BMI)
	}

	weightFile := filedef.NewWeight()
	weightFile.FileId = *fileID
	weightFile.UserProfile = up
	weightFile.WeightScales = append(weightFile.WeightScales, ws)

	fit := weightFile.ToFIT(nil)

	var buf bytes.Buffer
	if err := encoder.New(&buf).Encode(&fit); err != nil {
		return nil, fmt.Errorf("encode fit: %w", err)
	}
	return buf.Bytes(), nil
}
