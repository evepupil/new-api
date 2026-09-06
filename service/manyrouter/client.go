/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

package manyrouter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const ContractVersion = "m3-site-products-v1"
const maxResponseBytes = 2 << 20

var ErrUnavailable = errors.New("ManyRouter product data is unavailable")

type Product struct {
	Model              string   `json:"model"`
	Kind               string   `json:"kind"`
	StrategyKind       string   `json:"strategy_kind,omitempty"`
	DisplayName        string   `json:"display_name"`
	GroupKey           string   `json:"group_key"`
	EntryOpen          bool     `json:"entry_open"`
	SaleRatio          string   `json:"sale_ratio"`
	PriceVersionID     *string  `json:"price_version_id,omitempty"`
	PriceConfirmedAt   *string  `json:"price_confirmed_at,omitempty"`
	AvailableSuppliers int      `json:"available_suppliers"`
	FailoverReady      bool     `json:"failover_ready"`
	RequestSamples     int64    `json:"request_samples"`
	SLAPercent         *float64 `json:"sla_percent,omitempty"`
	TTFTP50Millis      *int64   `json:"ttft_p50_ms,omitempty"`
	TTFTP95Millis      *int64   `json:"ttft_p95_ms,omitempty"`
	QualityGrade       string   `json:"quality_grade"`
	Confidence         string   `json:"confidence"`
	FactsThrough       *string  `json:"facts_through,omitempty"`
	Status             string   `json:"status"`
}

type Snapshot struct {
	ID              string    `json:"id"`
	ContractVersion string    `json:"contract_version"`
	Version         int64     `json:"version"`
	SiteID          string    `json:"site_id"`
	SiteName        string    `json:"site_name"`
	RoutePlanID     string    `json:"route_plan_id"`
	ScoreRunID      *string   `json:"score_run_id,omitempty"`
	GeneratedAt     time.Time `json:"generated_at"`
	FactsThrough    *string   `json:"facts_through,omitempty"`
	Products        []Product `json:"products"`
	ContentHash     string    `json:"content_hash"`
}

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewFromEnvironment() (*Client, error) {
	return New(os.Getenv("MANYROUTER_URL"), os.Getenv("MANYROUTER_SITE_TOKEN"), nil)
}

func New(baseURL, token string, httpClient *http.Client) (*Client, error) {
	baseURL = strings.TrimSpace(baseURL)
	token = strings.TrimSpace(token)
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("%w: service address is invalid", ErrUnavailable)
	}
	if !strings.HasPrefix(token, "mrp_") || len(token) != 47 {
		return nil, fmt.Errorf("%w: site token is invalid", ErrUnavailable)
	}
	if httpClient == nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.MaxIdleConnsPerHost = 2
		transport.ResponseHeaderTimeout = 3 * time.Second
		httpClient = &http.Client{Transport: transport, Timeout: 4 * time.Second}
	}
	clientCopy := *httpClient
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, httpClient: &clientCopy}, nil
}

func (client *Client) Products(ctx context.Context) (Snapshot, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+"/api/v1/site/products", nil)
	if err != nil {
		return Snapshot{}, ErrUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Accept", "application/json")
	response, err := client.httpClient.Do(request)
	if err != nil {
		return Snapshot{}, ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return Snapshot{}, ErrUnavailable
	}
	limited := io.LimitReader(response.Body, maxResponseBytes+1)
	payload, err := io.ReadAll(limited)
	if err != nil || len(payload) > maxResponseBytes {
		return Snapshot{}, ErrUnavailable
	}
	var snapshot Snapshot
	if err := common.Unmarshal(payload, &snapshot); err != nil {
		return Snapshot{}, ErrUnavailable
	}
	if snapshot.ContractVersion != ContractVersion || snapshot.SiteID == "" || snapshot.RoutePlanID == "" ||
		snapshot.Version < 1 || snapshot.GeneratedAt.IsZero() || len(snapshot.ContentHash) != 64 {
		return Snapshot{}, ErrUnavailable
	}
	for _, product := range snapshot.Products {
		if product.Model == "" || product.GroupKey == "" || product.DisplayName == "" ||
			(product.Kind != "dedicated" && product.Kind != "fixed_auto") || product.AvailableSuppliers < 0 {
			return Snapshot{}, ErrUnavailable
		}
	}
	return snapshot, nil
}
