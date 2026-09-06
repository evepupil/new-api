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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	SyncContractVersion  = "m4-managed-sync-v1"
	SyncMaxChannels      = 100
	SyncMaxGroups        = 20
	SyncMaxModels        = 500
	SyncMaxGroupKeyBytes = 64
	SyncMaxRequestBytes  = int64(2 << 20)
)

var (
	ErrSyncInvalid  = errors.New("invalid ManyRouter managed sync request")
	operationIDExpr = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{7,127}$`)
)

func SyncCapabilities() (dto.ManyRouterSyncCapabilities, error) {
	basis, basisHash, err := SyncBillingBasis()
	if err != nil {
		return dto.ManyRouterSyncCapabilities{}, err
	}
	return dto.ManyRouterSyncCapabilities{
		ContractVersion:  SyncContractVersion,
		NewAPIVersion:    common.Version,
		DatabaseType:     string(common.MainDatabaseType()),
		BillingBasis:     basis,
		BillingBasisHash: basisHash,
		Features: dto.ManyRouterSyncFeatures{
			AtomicApply:           true,
			ManagedChannels:       true,
			MultipleGroups:        true,
			GroupRatios:           true,
			EntryVisibility:       true,
			PersistentIdempotency: true,
			FinalStateDigest:      true,
			LogRead:               true,
		},
		Limits: dto.ManyRouterSyncLimits{
			MaxChannels:      SyncMaxChannels,
			MaxGroups:        SyncMaxGroups,
			MaxModels:        SyncMaxModels,
			MaxGroupKeyBytes: SyncMaxGroupKeyBytes,
			MaxRequestBytes:  SyncMaxRequestBytes,
		},
		RetryPolicy: dto.ManyRouterSyncRetryPolicy{
			RetryTimes:  common.RetryTimes,
			StatusCodes: operation_setting.AutomaticRetryStatusCodesToString(),
		},
	}, nil
}

func ReadManagedState() (dto.ManyRouterManagedState, error) {
	state, err := model.ReadManyRouterManagedState()
	if err != nil {
		return dto.ManyRouterManagedState{}, err
	}
	_, basisHash, err := SyncBillingBasis()
	if err != nil {
		return dto.ManyRouterManagedState{}, err
	}
	state.BillingBasisHash = basisHash
	state.NewAPIVersion = common.Version
	return state, nil
}

func ApplyManagedState(request dto.ManyRouterApplyRequest) (dto.ManyRouterApplyResponse, error) {
	normalized, err := NormalizeApplyRequest(request)
	if err != nil {
		return dto.ManyRouterApplyResponse{}, err
	}
	return ApplyNormalizedManagedState(normalized)
}

func ApplyNormalizedManagedState(request dto.ManyRouterApplyRequest) (dto.ManyRouterApplyResponse, error) {
	payload, err := common.Marshal(request)
	if err != nil {
		return dto.ManyRouterApplyResponse{}, err
	}
	digest := sha256.Sum256(payload)
	response, err := model.ApplyManyRouterManagedState(request, hex.EncodeToString(digest[:]), time.Now())
	if err != nil {
		return dto.ManyRouterApplyResponse{}, err
	}
	_, basisHash, err := SyncBillingBasis()
	if err != nil {
		return dto.ManyRouterApplyResponse{}, err
	}
	response.State.BillingBasisHash = basisHash
	response.State.NewAPIVersion = common.Version
	return response, nil
}

func NormalizeApplyRequest(request dto.ManyRouterApplyRequest) (dto.ManyRouterApplyRequest, error) {
	if request.ContractVersion != SyncContractVersion {
		return dto.ManyRouterApplyRequest{}, syncInvalid("contract version is unsupported")
	}
	request.OperationID = strings.TrimSpace(request.OperationID)
	if !operationIDExpr.MatchString(request.OperationID) {
		return dto.ManyRouterApplyRequest{}, syncInvalid("operation ID is invalid")
	}
	if request.RoutePlanVersion < 1 {
		return dto.ManyRouterApplyRequest{}, syncInvalid("route plan version must be positive")
	}
	request.ExpectedStateHash = strings.ToLower(strings.TrimSpace(request.ExpectedStateHash))
	decodedHash, err := hex.DecodeString(request.ExpectedStateHash)
	if err != nil || len(decodedHash) != sha256.Size {
		return dto.ManyRouterApplyRequest{}, syncInvalid("expected state hash is invalid")
	}
	if len(request.Channels) > SyncMaxChannels || len(request.Groups) > SyncMaxGroups {
		return dto.ManyRouterApplyRequest{}, syncInvalid("managed state exceeds the supported resource limit")
	}

	groups := make(map[string]dto.ManyRouterManagedGroup, len(request.Groups))
	for index := range request.Groups {
		group := request.Groups[index]
		group.Key = strings.TrimSpace(group.Key)
		group.DisplayName = strings.TrimSpace(group.DisplayName)
		if !validManagedGroup(group.Key) || len(group.Key) > SyncMaxGroupKeyBytes {
			return dto.ManyRouterApplyRequest{}, syncInvalid("managed group key is invalid")
		}
		if group.DisplayName == "" || len(group.DisplayName) > 120 {
			return dto.ManyRouterApplyRequest{}, syncInvalid("managed group display name is invalid")
		}
		if _, exists := groups[group.Key]; exists {
			return dto.ManyRouterApplyRequest{}, syncInvalid("managed group keys must be unique")
		}
		ratio, err := decimal.NewFromString(strings.TrimSpace(group.SaleRatio))
		if err != nil || !ratio.IsPositive() || ratio.Exponent() < -6 || ratio.GreaterThan(decimal.RequireFromString("999999.999999")) {
			return dto.ManyRouterApplyRequest{}, syncInvalid("managed group sale ratio is invalid")
		}
		group.SaleRatio = ratio.String()
		groups[group.Key] = group
		request.Groups[index] = group
	}
	sort.Slice(request.Groups, func(i, j int) bool { return request.Groups[i].Key < request.Groups[j].Key })

	seenTags := make(map[string]bool, len(request.Channels))
	referencedDedicated := make(map[string]bool)
	totalModels := 0
	for index := range request.Channels {
		channel := request.Channels[index]
		channel.ManagedTag = strings.TrimSpace(channel.ManagedTag)
		relationID, err := relationIDFromManagedTag(channel.ManagedTag)
		if err != nil || seenTags[channel.ManagedTag] {
			return dto.ManyRouterApplyRequest{}, syncInvalid("managed channel tags must be valid and unique")
		}
		seenTags[channel.ManagedTag] = true
		channel.Name = strings.TrimSpace(channel.Name)
		if channel.Name == "" || len(channel.Name) > 120 {
			return dto.ManyRouterApplyRequest{}, syncInvalid("managed channel name is invalid")
		}
		channel.BaseURL, err = normalizeSyncBaseURL(channel.BaseURL)
		if err != nil {
			return dto.ManyRouterApplyRequest{}, err
		}
		if len(channel.APIKey) > 16384 || channel.CredentialVersion < 1 ||
			(channel.APIKey != "" && len(channel.APIKey) < 8) ||
			(channel.DesiredStatus == "enabled" && len(channel.APIKey) < 8) {
			return dto.ManyRouterApplyRequest{}, syncInvalid("managed channel credential is invalid")
		}
		if channel.DesiredStatus != "enabled" && channel.DesiredStatus != "disabled" {
			return dto.ManyRouterApplyRequest{}, syncInvalid("managed channel status is invalid")
		}
		if channel.Weight < 0 || channel.Weight > 1000 || channel.Priority < -1000000 || channel.Priority > 1000000 {
			return dto.ManyRouterApplyRequest{}, syncInvalid("managed channel priority or weight is invalid")
		}
		if len(channel.Models) == 0 {
			return dto.ManyRouterApplyRequest{}, syncInvalid("managed channel must contain at least one model")
		}
		totalModels += len(channel.Models)
		if totalModels > SyncMaxModels {
			return dto.ManyRouterApplyRequest{}, syncInvalid("managed state contains too many models")
		}
		seenModels := make(map[string]bool, len(channel.Models))
		for modelIndex := range channel.Models {
			item := channel.Models[modelIndex]
			item.Model = strings.TrimSpace(item.Model)
			item.UpstreamModel = strings.TrimSpace(item.UpstreamModel)
			if !validSyncModelName(item.Model) || !validSyncModelName(item.UpstreamModel) || seenModels[item.Model] {
				return dto.ManyRouterApplyRequest{}, syncInvalid("managed channel models are invalid or duplicated")
			}
			seenModels[item.Model] = true
			channel.Models[modelIndex] = item
		}
		sort.Slice(channel.Models, func(i, j int) bool { return channel.Models[i].Model < channel.Models[j].Model })

		expectedDedicated := "mr_s_" + strings.ReplaceAll(relationID.String(), "-", "")
		seenChannelGroups := make(map[string]bool, len(channel.Groups))
		containsDedicated := false
		for groupIndex := range channel.Groups {
			key := strings.TrimSpace(channel.Groups[groupIndex])
			if !validManagedGroup(key) || seenChannelGroups[key] {
				return dto.ManyRouterApplyRequest{}, syncInvalid("managed channel groups are invalid or duplicated")
			}
			if strings.HasPrefix(key, "mr_s_") && key != expectedDedicated {
				return dto.ManyRouterApplyRequest{}, syncInvalid("managed channel contains another relation's dedicated group")
			}
			if _, exists := groups[key]; !exists {
				return dto.ManyRouterApplyRequest{}, syncInvalid("managed channel references an undeclared group")
			}
			seenChannelGroups[key] = true
			channel.Groups[groupIndex] = key
			containsDedicated = containsDedicated || key == expectedDedicated
		}
		if !containsDedicated {
			return dto.ManyRouterApplyRequest{}, syncInvalid("managed channel is missing its dedicated group")
		}
		referencedDedicated[expectedDedicated] = true
		sort.Strings(channel.Groups)
		request.Channels[index] = channel
	}
	for key := range groups {
		if strings.HasPrefix(key, "mr_s_") && !referencedDedicated[key] {
			return dto.ManyRouterApplyRequest{}, syncInvalid("dedicated group has no matching managed channel")
		}
	}
	sort.Slice(request.Channels, func(i, j int) bool {
		return request.Channels[i].ManagedTag < request.Channels[j].ManagedTag
	})
	return request, nil
}

func normalizeSyncBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", syncInvalid("managed channel base URL is invalid")
	}
	return strings.TrimRight(raw, "/"), nil
}

func relationIDFromManagedTag(tag string) (uuid.UUID, error) {
	if !strings.HasPrefix(tag, "manyrouter:") {
		return uuid.Nil, errors.New("managed tag prefix is invalid")
	}
	parsed, err := uuid.Parse(strings.TrimPrefix(tag, "manyrouter:"))
	if err != nil || tag != "manyrouter:"+parsed.String() {
		return uuid.Nil, errors.New("managed tag UUID is invalid")
	}
	return parsed, nil
}

func validManagedGroup(group string) bool {
	switch group {
	case "mrap", "mral", "mras", "mraq", "mrab":
		return true
	}
	if !strings.HasPrefix(group, "mr_s_") || len(group) != 37 {
		return false
	}
	decoded, err := hex.DecodeString(strings.TrimPrefix(group, "mr_s_"))
	return err == nil && len(decoded) == 16
}

func validSyncModelName(value string) bool {
	return value != "" && len(value) <= 191 && !strings.ContainsAny(value, ",\r\n")
}

func syncInvalid(message string) error {
	return fmt.Errorf("%w: %s", ErrSyncInvalid, message)
}
