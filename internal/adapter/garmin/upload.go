package garmin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// uploadResult mirrors the subset of Garmin's detailedImportResult we care
// about (mechanism doc §6).
type uploadResult struct {
	UploadID  int64
	Duplicate bool
}

// uploadFIT posts a binary FIT file to the upload-service. Both 2xx and 409
// (duplicate upload) are treated as success (mechanism doc §6).
func uploadFIT(httpClient *http.Client, bearer string, fitBytes []byte) (*uploadResult, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreatePart(map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name="file"; filename="%s_sync.fit"`, time.Now().UTC().Format("20060102150405"))},
		"Content-Type":        {"application/octet-stream"},
	})
	if err != nil {
		return nil, fmt.Errorf("create multipart part: %w", err)
	}
	if _, err := part.Write(fitBytes); err != nil {
		return nil, fmt.Errorf("write fit bytes: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close multipart writer: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, uploadURL, &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("NK", "NT")
	req.Header.Set("origin", "https://sso.garmin.com")
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upload response: %w", err)
	}

	if resp.StatusCode != http.StatusConflict && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		return nil, fmt.Errorf("upload failed: status %d: %s", resp.StatusCode, string(respBody))
	}

	return parseUploadResponse(respBody)
}

type uploadResponseEnvelope struct {
	DetailedImportResult struct {
		UploadID  int64 `json:"uploadId"`
		Successes []struct {
			InternalID int64 `json:"internalId"`
		} `json:"successes"`
		Failures []struct {
			Messages []struct {
				Code    int    `json:"code"`
				Content string `json:"content"`
			} `json:"messages"`
		} `json:"failures"`
	} `json:"detailedImportResult"`
}

// duplicateMessageCode is Garmin's "activity already uploaded" code (§6) —
// a benign failure that should not be treated as an error.
const duplicateMessageCode = 202

func parseUploadResponse(body []byte) (*uploadResult, error) {
	// Some 409 responses come back empty; that's still a benign duplicate.
	if len(body) == 0 {
		return &uploadResult{Duplicate: true}, nil
	}

	var envelope uploadResponseEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("parse upload response: %w", err)
	}

	result := &uploadResult{UploadID: envelope.DetailedImportResult.UploadID}
	if len(envelope.DetailedImportResult.Successes) > 0 {
		return result, nil
	}

	for _, failure := range envelope.DetailedImportResult.Failures {
		for _, msg := range failure.Messages {
			if msg.Code == duplicateMessageCode {
				result.Duplicate = true
				return result, nil
			}
			return nil, fmt.Errorf("upload rejected: code %d: %s", msg.Code, msg.Content)
		}
	}

	// No successes and no failures reported — treat as success (2xx status).
	return result, nil
}
