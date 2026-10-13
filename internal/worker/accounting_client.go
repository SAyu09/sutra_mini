package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// PostVoucherRequest is the JSON payload sent to the accounting service.
type PostVoucherRequest struct {
	ClientRef   string `json:"client_ref"`
	AmountPaise int64  `json:"amount_paise"`
	Type        string `json:"type"`
	SourceID    int64  `json:"source_id"`
}

// PostVoucherStatus indicates the classification of the post response.
type PostVoucherStatus int

const (
	PostStatusOK PostVoucherStatus = iota
	PostStatusConflictReconciled
	PostStatusTransientError
)

// PostVoucherResult represents the parsed result from the accounting API.
type PostVoucherResult struct {
	Status       PostVoucherStatus
	ExternalRef  string
	StatusCode   int
	ErrorMessage string
}

// AccountingClient defines the contract for sending vouchers to the external accounting system.
type AccountingClient interface {
	PostVoucher(ctx context.Context, req PostVoucherRequest) (PostVoucherResult, error)
}

// HTTPAccountingClient is an implementation of AccountingClient talking over HTTP.
type HTTPAccountingClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewHTTPAccountingClient creates a new HTTPAccountingClient with the configured baseURL and timeout.
func NewHTTPAccountingClient(baseURL string, timeout time.Duration) *HTTPAccountingClient {
	baseURL = strings.TrimRight(baseURL, "/")
	return &HTTPAccountingClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// PostVoucher posts a voucher to the accounting server and handles 200, 409 reconciliation, and errors.
func (c *HTTPAccountingClient) PostVoucher(ctx context.Context, req PostVoucherRequest) (PostVoucherResult, error) {
	url := c.baseURL + "/v1/accounting/vouchers"

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return PostVoucherResult{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return PostVoucherResult{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return PostVoucherResult{
			Status:       PostStatusTransientError,
			ErrorMessage: err.Error(),
		}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return PostVoucherResult{
			Status:       PostStatusTransientError,
			StatusCode:   resp.StatusCode,
			ErrorMessage: fmt.Sprintf("read response: %v", err),
		}, err
	}

	var jsonResp struct {
		ExternalRef string `json:"external_ref"`
		Error       string `json:"error"`
	}
	_ = json.Unmarshal(respBody, &jsonResp)

	switch resp.StatusCode {
	case http.StatusOK:
		if jsonResp.ExternalRef == "" {
			return PostVoucherResult{
				Status:       PostStatusTransientError,
				StatusCode:   resp.StatusCode,
				ErrorMessage: "missing external_ref in 200 OK response",
			}, errors.New("missing external_ref in 200 response")
		}
		return PostVoucherResult{
			Status:      PostStatusOK,
			StatusCode:  resp.StatusCode,
			ExternalRef: jsonResp.ExternalRef,
		}, nil

	case http.StatusConflict:
		// 409 Conflict Reconciliation: treat as success!
		if jsonResp.ExternalRef == "" {
			return PostVoucherResult{
				Status:       PostStatusTransientError,
				StatusCode:   resp.StatusCode,
				ErrorMessage: "missing external_ref in 409 Conflict response",
			}, errors.New("missing external_ref in 409 response")
		}
		return PostVoucherResult{
			Status:      PostStatusConflictReconciled,
			StatusCode:  resp.StatusCode,
			ExternalRef: jsonResp.ExternalRef,
		}, nil

	default:
		errMsg := jsonResp.Error
		if errMsg == "" {
			errMsg = strings.TrimSpace(string(respBody))
		}
		if errMsg == "" {
			errMsg = fmt.Sprintf("HTTP %d", resp.StatusCode)
		} else {
			errMsg = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, errMsg)
		}
		return PostVoucherResult{
			Status:       PostStatusTransientError,
			StatusCode:   resp.StatusCode,
			ErrorMessage: errMsg,
		}, fmt.Errorf("accounting service returned status %d: %s", resp.StatusCode, errMsg)
	}
}
