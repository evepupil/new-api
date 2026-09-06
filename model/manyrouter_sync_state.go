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

package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func ensureManyRouterOptions(tx *gorm.DB) error {
	defaults := map[string]string{
		"GroupRatio":       ratio_setting.GroupRatio2JSONString(),
		"UserUsableGroups": setting.UserUsableGroups2JSONString(),
	}
	for _, key := range []string{"GroupRatio", "UserUsableGroups"} {
		option := Option{Key: key, Value: defaults[key]}
		if err := tx.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
			return err
		}
	}
	return nil
}

func readManyRouterManagedState(db *gorm.DB, locked bool) (dto.ManyRouterManagedState, error) {
	query := db.Where("tag LIKE ?", "manyrouter:%").Order("id")
	if locked {
		query = lockForUpdate(query)
	}
	var channels []Channel
	if err := query.Find(&channels).Error; err != nil {
		return dto.ManyRouterManagedState{}, err
	}
	ratioValues, userGroups, err := readManyRouterGroupOptions(db)
	if err != nil {
		return dto.ManyRouterManagedState{}, err
	}

	state := dto.ManyRouterManagedState{
		ContractVersion: "m4-managed-sync-v1",
		Channels:        make([]dto.ManyRouterManagedChannel, 0, len(channels)),
		Groups:          make([]dto.ManyRouterManagedGroup, 0),
		Conflicts:       make([]string, 0),
	}
	seenTags := make(map[string]bool, len(channels))
	managedGroups := make(map[string]bool)
	for index := range channels {
		channel := &channels[index]
		tag := strings.TrimSpace(manyRouterStringValue(channel.Tag))
		if !isManyRouterManagedTag(tag) {
			state.Conflicts = append(state.Conflicts, fmt.Sprintf("channel %d has an invalid managed tag", channel.Id))
			continue
		}
		if seenTags[tag] {
			state.Conflicts = append(state.Conflicts, fmt.Sprintf("managed tag %s belongs to multiple channels", tag))
			continue
		}
		seenTags[tag] = true
		models, mappingErr := manyRouterChannelModels(channel.Models, manyRouterStringValue(channel.ModelMapping))
		if mappingErr != nil {
			state.Conflicts = append(state.Conflicts, fmt.Sprintf("channel %d has invalid model mapping", channel.Id))
			continue
		}
		groups := splitManyRouterCSV(channel.Group)
		for _, group := range groups {
			if isManyRouterManagedGroup(group) {
				managedGroups[group] = true
			}
		}
		state.Channels = append(state.Channels, dto.ManyRouterManagedChannel{
			ID:                int64(channel.Id),
			ManagedTag:        tag,
			Name:              channel.Name,
			BaseURL:           strings.TrimRight(channel.GetBaseURL(), "/"),
			CredentialVersion: channel.ChannelInfo.ManyRouterCredentialVersion,
			Models:            models,
			Groups:            groups,
			Priority:          channel.GetPriority(),
			Weight:            channel.GetWeight(),
			Status:            manyRouterChannelStatus(channel.Status),
		})
	}
	for key := range ratioValues {
		if isManyRouterManagedGroup(key) {
			managedGroups[key] = true
		}
	}
	for key := range userGroups {
		if isManyRouterManagedGroup(key) {
			managedGroups[key] = true
		}
	}
	for key := range managedGroups {
		rawRatio, exists := ratioValues[key]
		if !exists || len(bytes.TrimSpace(rawRatio)) == 0 {
			state.Conflicts = append(state.Conflicts, fmt.Sprintf("managed group %s has no price", key))
			continue
		}
		displayName, visible := userGroups[key]
		if displayName == "" {
			displayName = key
		}
		state.Groups = append(state.Groups, dto.ManyRouterManagedGroup{
			Key:         key,
			DisplayName: displayName,
			SaleRatio:   string(bytes.TrimSpace(rawRatio)),
			Visible:     visible,
		})
	}
	sort.Slice(state.Channels, func(i, j int) bool {
		return state.Channels[i].ManagedTag < state.Channels[j].ManagedTag
	})
	sort.Slice(state.Groups, func(i, j int) bool { return state.Groups[i].Key < state.Groups[j].Key })
	sort.Strings(state.Conflicts)
	hash, err := manyRouterStateHash(state)
	if err != nil {
		return dto.ManyRouterManagedState{}, err
	}
	state.StateHash = hash
	return state, nil
}

func readManyRouterGroupOptions(db *gorm.DB) (map[string]json.RawMessage, map[string]string, error) {
	var options []Option
	if err := db.Where(map[string]any{"key": []string{"GroupRatio", "UserUsableGroups"}}).Find(&options).Error; err != nil {
		return nil, nil, err
	}
	ratioJSON := ratio_setting.GroupRatio2JSONString()
	userJSON := setting.UserUsableGroups2JSONString()
	for _, option := range options {
		switch option.Key {
		case "GroupRatio":
			ratioJSON = option.Value
		case "UserUsableGroups":
			userJSON = option.Value
		}
	}
	ratios := make(map[string]json.RawMessage)
	if err := common.UnmarshalJsonStr(ratioJSON, &ratios); err != nil {
		return nil, nil, err
	}
	groups := make(map[string]string)
	if err := common.UnmarshalJsonStr(userJSON, &groups); err != nil {
		return nil, nil, err
	}
	return ratios, groups, nil
}

func manyRouterStateHash(state dto.ManyRouterManagedState) (string, error) {
	canonical := struct {
		Channels  []dto.ManyRouterManagedChannel `json:"channels"`
		Groups    []dto.ManyRouterManagedGroup   `json:"groups"`
		Conflicts []string                       `json:"conflicts"`
	}{Channels: state.Channels, Groups: state.Groups, Conflicts: state.Conflicts}
	payload, err := common.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func manyRouterChannelModels(modelsCSV, mappingJSON string) ([]dto.ManyRouterSyncModel, error) {
	mapping := make(map[string]string)
	if strings.TrimSpace(mappingJSON) != "" {
		if err := common.UnmarshalJsonStr(mappingJSON, &mapping); err != nil {
			return nil, err
		}
	}
	models := splitManyRouterCSV(modelsCSV)
	result := make([]dto.ManyRouterSyncModel, 0, len(models))
	for _, modelName := range models {
		upstream := mapping[modelName]
		if upstream == "" {
			upstream = modelName
		}
		result = append(result, dto.ManyRouterSyncModel{Model: modelName, UpstreamModel: upstream})
	}
	return result, nil
}

func encodeManyRouterModels(models []dto.ManyRouterSyncModel) (string, string, error) {
	names := make([]string, 0, len(models))
	mapping := make(map[string]string)
	for _, item := range models {
		names = append(names, item.Model)
		if item.Model != item.UpstreamModel {
			mapping[item.Model] = item.UpstreamModel
		}
	}
	mappingJSON := ""
	if len(mapping) > 0 {
		encoded, err := common.Marshal(mapping)
		if err != nil {
			return "", "", err
		}
		mappingJSON = string(encoded)
	}
	return strings.Join(names, ","), mappingJSON, nil
}

func splitManyRouterCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func isManyRouterManagedTag(tag string) bool {
	if !strings.HasPrefix(tag, "manyrouter:") {
		return false
	}
	parsed, err := uuid.Parse(strings.TrimPrefix(tag, "manyrouter:"))
	return err == nil && tag == "manyrouter:"+parsed.String()
}

func isManyRouterManagedGroup(group string) bool {
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

func manyRouterChannelStatus(status int) string {
	switch status {
	case common.ChannelStatusEnabled:
		return "enabled"
	case common.ChannelStatusManuallyDisabled:
		return "manually_disabled"
	case common.ChannelStatusAutoDisabled:
		return "auto_disabled"
	default:
		return "unknown"
	}
}

func manyRouterStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
