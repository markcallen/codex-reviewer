package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (c Client) Report(ctx context.Context, reportURL string) ([]byte, error) {
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
	u, err := url.JoinPath(strings.TrimRight(c.BaseURL, "/"), strings.TrimLeft(reportURL, "/"))
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("review report returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func (c Client) WaitReport(ctx context.Context, reportURL string, interval time.Duration) ([]byte, error) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		report, err := c.Report(ctx, reportURL)
		if err == nil {
			return report, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c Client) Submit(ctx context.Context, req ReviewRequest) (ReviewResponse, error) {
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
	baseURL, err := url.JoinPath(strings.TrimRight(c.BaseURL, "/"), "reviews")
	if err != nil {
		return ReviewResponse{}, err
	}
	data, err := req.JSON()
	if err != nil {
		return ReviewResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(data))
	if err != nil {
		return ReviewResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return ReviewResponse{}, err
	}
	defer resp.Body.Close()
	var review ReviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&review); err != nil {
		return ReviewResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if review.Error != "" {
			return review, fmt.Errorf("review API returned %s: %s", resp.Status, review.Error)
		}
		return review, fmt.Errorf("review API returned %s", resp.Status)
	}
	return review, nil
}
