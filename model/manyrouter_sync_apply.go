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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
)

func applyManyRouterChannels(tx *gorm.DB, desired []dto.ManyRouterDesiredChannel) ([]dto.ManyRouterSyncAction, error) {
	var existing []Channel
	if err := lockForUpdate(tx).Where("tag LIKE ?", "manyrouter:%").Order("id").Find(&existing).Error; err != nil {
		return nil, err
	}
	byTag := make(map[string]*Channel, len(existing))
	for index := range existing {
		channel := &existing[index]
		tag := manyRouterStringValue(channel.Tag)
		if !isManyRouterManagedTag(tag) || byTag[tag] != nil {
			return nil, fmt.Errorf("%w: managed channel tags are invalid or duplicated", ErrManyRouterOwnershipConflict)
		}
		byTag[tag] = channel
	}
	actions := make([]dto.ManyRouterSyncAction, 0, len(existing)+len(desired))
	desiredTags := make(map[string]bool, len(desired))
	for _, item := range desired {
		desiredTags[item.ManagedTag] = true
		channel, exists := byTag[item.ManagedTag]
		status := common.ChannelStatusManuallyDisabled
		if item.DesiredStatus == "enabled" {
			status = common.ChannelStatusEnabled
		}
		if exists && channel.Status == common.ChannelStatusManuallyDisabled && item.DesiredStatus == "enabled" && !item.Resume {
			status = common.ChannelStatusManuallyDisabled
		}
		models, modelMapping, err := encodeManyRouterModels(item.Models)
		if err != nil {
			return nil, err
		}
		groupList := strings.Join(item.Groups, ",")
		baseURL := item.BaseURL
		priority := item.Priority
		weight := uint(item.Weight)
		tag := item.ManagedTag
		autoBan := 1
		if !exists {
			if item.APIKey == "" && item.DesiredStatus == "disabled" {
				actions = append(actions, dto.ManyRouterSyncAction{Resource: "channel", Key: tag, Action: "absent"})
				continue
			}
			channel = &Channel{
				Type:          1,
				Key:           item.APIKey,
				Status:        status,
				Name:          item.Name,
				Weight:        &weight,
				BaseURL:       &baseURL,
				Models:        models,
				Group:         groupList,
				ModelMapping:  &modelMapping,
				Priority:      &priority,
				AutoBan:       &autoBan,
				Tag:           &tag,
				OtherSettings: "{}",
				ChannelInfo:   ChannelInfo{ManyRouterCredentialVersion: item.CredentialVersion},
			}
			if err := tx.Create(channel).Error; err != nil {
				return nil, err
			}
			if err := channel.AddAbilities(tx); err != nil {
				return nil, err
			}
			actions = append(actions, dto.ManyRouterSyncAction{Resource: "channel", Key: tag, Action: "created", ChannelID: int64(channel.Id)})
			continue
		}
		if channel.Type != 1 {
			return nil, fmt.Errorf("%w: managed tag %s belongs to an unsupported channel", ErrManyRouterOwnershipConflict, tag)
		}
		info := channel.ChannelInfo
		if item.APIKey != "" {
			info.ManyRouterCredentialVersion = item.CredentialVersion
		}
		keyMatches := item.APIKey == "" || channel.Key == item.APIKey
		matches := keyMatches && channel.Name == item.Name &&
			strings.TrimRight(channel.GetBaseURL(), "/") == item.BaseURL && channel.Models == models &&
			channel.Group == groupList && manyRouterStringValue(channel.ModelMapping) == modelMapping &&
			channel.GetPriority() == priority && channel.GetWeight() == item.Weight && channel.Status == status &&
			channel.ChannelInfo.ManyRouterCredentialVersion == item.CredentialVersion
		if matches {
			actions = append(actions, dto.ManyRouterSyncAction{Resource: "channel", Key: tag, Action: "unchanged", ChannelID: int64(channel.Id)})
			continue
		}
		updates := map[string]any{
			"status": status, "name": item.Name, "weight": weight,
			"base_url": baseURL, "models": models, "group": groupList, "model_mapping": modelMapping,
			"priority": priority, "auto_ban": autoBan, "channel_info": info,
		}
		if item.APIKey != "" {
			updates["key"] = item.APIKey
		}
		if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Updates(updates).Error; err != nil {
			return nil, err
		}
		if item.APIKey != "" {
			channel.Key = item.APIKey
		}
		channel.Status = status
		channel.Name = item.Name
		channel.Weight = &weight
		channel.BaseURL = &baseURL
		channel.Models = models
		channel.Group = groupList
		channel.ModelMapping = &modelMapping
		channel.Priority = &priority
		channel.AutoBan = &autoBan
		channel.ChannelInfo = info
		if err := channel.UpdateAbilities(tx); err != nil {
			return nil, err
		}
		action := "updated"
		if status == common.ChannelStatusManuallyDisabled && item.DesiredStatus == "enabled" && !item.Resume {
			action = "manual_lock_preserved"
		}
		actions = append(actions, dto.ManyRouterSyncAction{Resource: "channel", Key: tag, Action: action, ChannelID: int64(channel.Id)})
	}
	for index := range existing {
		channel := &existing[index]
		tag := manyRouterStringValue(channel.Tag)
		if desiredTags[tag] || channel.Status == common.ChannelStatusManuallyDisabled {
			continue
		}
		if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Update("status", common.ChannelStatusManuallyDisabled).Error; err != nil {
			return nil, err
		}
		channel.Status = common.ChannelStatusManuallyDisabled
		if err := channel.UpdateAbilities(tx); err != nil {
			return nil, err
		}
		actions = append(actions, dto.ManyRouterSyncAction{Resource: "channel", Key: tag, Action: "disabled", ChannelID: int64(channel.Id)})
	}
	return actions, nil
}

func applyManyRouterGroups(tx *gorm.DB, desired []dto.ManyRouterManagedGroup) ([]dto.ManyRouterSyncAction, map[string]string, error) {
	ratios, userGroups, err := readManyRouterGroupOptions(tx)
	if err != nil {
		return nil, nil, err
	}
	oldRatios := make(map[string]string)
	oldNames := make(map[string]string)
	for key, value := range ratios {
		if isManyRouterManagedGroup(key) {
			oldRatios[key] = string(bytes.TrimSpace(value))
			delete(ratios, key)
		}
	}
	for key, value := range userGroups {
		if isManyRouterManagedGroup(key) {
			oldNames[key] = value
			delete(userGroups, key)
		}
	}
	actions := make([]dto.ManyRouterSyncAction, 0, len(desired)+len(oldRatios))
	desiredKeys := make(map[string]bool, len(desired))
	for _, group := range desired {
		desiredKeys[group.Key] = true
		ratios[group.Key] = json.RawMessage(group.SaleRatio)
		if group.Visible {
			userGroups[group.Key] = group.DisplayName
		}
		action := "created"
		oldRatio, existed := oldRatios[group.Key]
		oldName, wasVisible := oldNames[group.Key]
		if existed {
			action = "updated"
			if oldRatio == group.SaleRatio && wasVisible == group.Visible && (!group.Visible || oldName == group.DisplayName) {
				action = "unchanged"
			}
		}
		actions = append(actions, dto.ManyRouterSyncAction{Resource: "group", Key: group.Key, Action: action})
	}
	for key := range oldRatios {
		if !desiredKeys[key] {
			actions = append(actions, dto.ManyRouterSyncAction{Resource: "group", Key: key, Action: "removed"})
		}
	}
	ratioJSON, err := common.Marshal(ratios)
	if err != nil {
		return nil, nil, err
	}
	userJSON, err := common.Marshal(userGroups)
	if err != nil {
		return nil, nil, err
	}
	values := map[string]string{"GroupRatio": string(ratioJSON), "UserUsableGroups": string(userJSON)}
	for _, key := range []string{"GroupRatio", "UserUsableGroups"} {
		option := Option{Key: key}
		if err := tx.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
			return nil, nil, err
		}
		option.Value = values[key]
		if err := tx.Save(&option).Error; err != nil {
			return nil, nil, err
		}
	}
	return actions, values, nil
}
