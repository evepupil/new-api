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

package dto

import "encoding/json"

type ManyRouterSyncCapabilities struct {
	ContractVersion  string                     `json:"contract_version"`
	NewAPIVersion    string                     `json:"new_api_version"`
	DatabaseType     string                     `json:"database_type"`
	BillingBasis     map[string]json.RawMessage `json:"billing_basis"`
	BillingBasisHash string                     `json:"billing_basis_hash"`
	Features         ManyRouterSyncFeatures     `json:"features"`
	Limits           ManyRouterSyncLimits       `json:"limits"`
	RetryPolicy      ManyRouterSyncRetryPolicy  `json:"retry_policy"`
}

type ManyRouterSyncFeatures struct {
	AtomicApply           bool `json:"atomic_apply"`
	ManagedChannels       bool `json:"managed_channels"`
	MultipleGroups        bool `json:"multiple_groups"`
	GroupRatios           bool `json:"group_ratios"`
	EntryVisibility       bool `json:"entry_visibility"`
	PersistentIdempotency bool `json:"persistent_idempotency"`
	FinalStateDigest      bool `json:"final_state_digest"`
	LogRead               bool `json:"log_read"`
}

type ManyRouterSyncLimits struct {
	MaxChannels      int   `json:"max_channels"`
	MaxGroups        int   `json:"max_groups"`
	MaxModels        int   `json:"max_models"`
	MaxGroupKeyBytes int   `json:"max_group_key_bytes"`
	MaxRequestBytes  int64 `json:"max_request_bytes"`
}

type ManyRouterSyncRetryPolicy struct {
	RetryTimes  int    `json:"retry_times"`
	StatusCodes string `json:"status_codes"`
}

type ManyRouterSyncModel struct {
	Model         string `json:"model"`
	UpstreamModel string `json:"upstream_model"`
}

type ManyRouterDesiredChannel struct {
	ManagedTag        string                `json:"managed_tag"`
	Name              string                `json:"name"`
	BaseURL           string                `json:"base_url"`
	APIKey            string                `json:"api_key"`
	CredentialVersion int                   `json:"credential_version"`
	Models            []ManyRouterSyncModel `json:"models"`
	Groups            []string              `json:"groups"`
	Priority          int64                 `json:"priority"`
	Weight            int                   `json:"weight"`
	DesiredStatus     string                `json:"desired_status"`
	Resume            bool                  `json:"resume"`
}

type ManyRouterManagedChannel struct {
	ID                int64                 `json:"id"`
	ManagedTag        string                `json:"managed_tag"`
	Name              string                `json:"name"`
	BaseURL           string                `json:"base_url"`
	CredentialVersion int                   `json:"credential_version"`
	Models            []ManyRouterSyncModel `json:"models"`
	Groups            []string              `json:"groups"`
	Priority          int64                 `json:"priority"`
	Weight            int                   `json:"weight"`
	Status            string                `json:"status"`
}

type ManyRouterManagedGroup struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	SaleRatio   string `json:"sale_ratio"`
	Visible     bool   `json:"visible"`
}

type ManyRouterManagedState struct {
	ContractVersion  string                     `json:"contract_version"`
	NewAPIVersion    string                     `json:"new_api_version"`
	StateHash        string                     `json:"state_hash"`
	BillingBasisHash string                     `json:"billing_basis_hash"`
	Channels         []ManyRouterManagedChannel `json:"channels"`
	Groups           []ManyRouterManagedGroup   `json:"groups"`
	Conflicts        []string                   `json:"conflicts"`
}

type ManyRouterApplyRequest struct {
	ContractVersion   string                     `json:"contract_version"`
	OperationID       string                     `json:"operation_id"`
	RoutePlanVersion  int64                      `json:"route_plan_version"`
	ExpectedStateHash string                     `json:"expected_state_hash"`
	Channels          []ManyRouterDesiredChannel `json:"channels"`
	Groups            []ManyRouterManagedGroup   `json:"groups"`
}

type ManyRouterSyncAction struct {
	Resource  string `json:"resource"`
	Key       string `json:"key"`
	Action    string `json:"action"`
	ChannelID int64  `json:"channel_id,omitempty"`
}

type ManyRouterApplyResponse struct {
	OperationID      string                 `json:"operation_id"`
	RoutePlanVersion int64                  `json:"route_plan_version"`
	Replayed         bool                   `json:"replayed"`
	Actions          []ManyRouterSyncAction `json:"actions"`
	State            ManyRouterManagedState `json:"state"`
}

type ManyRouterSyncLog struct {
	ID                int64  `json:"id"`
	CreatedAt         int64  `json:"created_at"`
	Type              int    `json:"type"`
	Content           string `json:"content"`
	Model             string `json:"model_name"`
	InputTokens       int64  `json:"prompt_tokens"`
	OutputTokens      int64  `json:"completion_tokens"`
	DurationSeconds   int64  `json:"use_time"`
	Stream            bool   `json:"is_stream"`
	ChannelID         int64  `json:"channel"`
	Group             string `json:"group"`
	RequestID         string `json:"request_id,omitempty"`
	UpstreamRequestID string `json:"upstream_request_id,omitempty"`
	Other             string `json:"other"`
}

type ManyRouterSyncLogPage struct {
	Items    []ManyRouterSyncLog `json:"items"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"page_size"`
}
