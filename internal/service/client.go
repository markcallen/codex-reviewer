package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
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
