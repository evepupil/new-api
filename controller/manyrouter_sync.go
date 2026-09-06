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

package controller

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	manyrouterservice "github.com/QuantumNous/new-api/service/manyrouter"
	"github.com/gin-gonic/gin"
)

func GetManyRouterSyncCapabilities(c *gin.Context) {
	capabilities, err := manyrouterservice.SyncCapabilities()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "ManyRouter sync capabilities are unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": capabilities})
}

func GetManyRouterManagedState(c *gin.Context) {
	state, err := manyrouterservice.ReadManagedState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Unable to read ManyRouter managed state"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": state})
}

func ApplyManyRouterManagedState(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, manyrouterservice.SyncMaxRequestBytes)
	var request dto.ManyRouterApplyRequest
	if err := common.DecodeJsonStrict(c.Request.Body, &request); err != nil {
		status := http.StatusBadRequest
		message := "ManyRouter managed state request is invalid"
		if common.IsRequestBodyTooLargeError(err) {
			status = http.StatusRequestEntityTooLarge
			message = "ManyRouter managed state request is too large"
		}
		c.JSON(status, gin.H{"success": false, "message": message})
		return
	}
	normalized, err := manyrouterservice.NormalizeApplyRequest(request)
	if err != nil {
		writeManyRouterSyncError(c, err)
		return
	}
	current, err := manyrouterservice.ReadManagedState()
	if err != nil {
		writeManyRouterSyncError(c, err)
		return
	}
	if current.StateHash == normalized.ExpectedStateHash {
		if err := preflightManyRouterChannels(c, current, normalized); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": err.Error()})
			return
		}
	}
	response, err := manyrouterservice.ApplyNormalizedManagedState(normalized)
	if err != nil {
		writeManyRouterSyncError(c, err)
		return
	}
	if !response.Replayed {
		model.RecordOperationAuditLog(0, "Applied ManyRouter managed state", c.ClientIP(), "manyrouter.sync.apply", map[string]interface{}{
			"operation_id":       response.OperationID,
			"route_plan_version": response.RoutePlanVersion,
			"action_count":       len(response.Actions),
			"state_hash":         response.State.StateHash,
		}, map[string]interface{}{"auth_method": "manyrouter_sync_token"}, nil)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": response})
}

func preflightManyRouterChannels(
	c *gin.Context,
	current dto.ManyRouterManagedState,
	request dto.ManyRouterApplyRequest,
) error {
	currentByTag := make(map[string]dto.ManyRouterManagedChannel, len(current.Channels))
	for _, channel := range current.Channels {
		currentByTag[channel.ManagedTag] = channel
	}
	testUserID := 0
	for _, desired := range request.Channels {
		if desired.DesiredStatus != "enabled" {
			continue
		}
		actual, exists := currentByTag[desired.ManagedTag]
		if exists && actual.Status == "manually_disabled" && !desired.Resume {
			continue
		}
		if exists && actual.Status == "enabled" && manyRouterChannelPreflightMatches(actual, desired) {
			continue
		}
		if testUserID == 0 {
			var err error
			testUserID, err = resolveChannelTestUserID(c)
			if err != nil {
				return errors.New("ManyRouter channel preflight user is unavailable")
			}
		}
		models := make([]string, 0, len(desired.Models))
		mapping := make(map[string]string)
		for _, item := range desired.Models {
			models = append(models, item.Model)
			if item.Model != item.UpstreamModel {
				mapping[item.Model] = item.UpstreamModel
			}
		}
		mappingJSON := ""
		if len(mapping) > 0 {
			encoded, err := common.Marshal(mapping)
			if err != nil {
				return errors.New("ManyRouter channel preflight configuration is invalid")
			}
			mappingJSON = string(encoded)
		}
		baseURL := desired.BaseURL
		priority := desired.Priority
		weight := uint(desired.Weight)
		autoBan := 1
		tag := desired.ManagedTag
		channelID := 0
		if exists {
			channelID = int(actual.ID)
		}
		channel := model.Channel{
			Id: channelID, Type: constant.ChannelTypeOpenAI, Key: desired.APIKey,
			Status: common.ChannelStatusEnabled, Name: desired.Name, Weight: &weight,
			BaseURL: &baseURL, Models: strings.Join(models, ","), Group: strings.Join(desired.Groups, ","),
			ModelMapping: &mappingJSON, Priority: &priority, AutoBan: &autoBan, Tag: &tag,
			ChannelInfo: model.ChannelInfo{ManyRouterCredentialVersion: desired.CredentialVersion},
		}
		result := testChannel(c.Request.Context(), &channel, testUserID, desired.Models[0].Model, "", false)
		if result.localErr != nil {
			common.SysLog(fmt.Sprintf(
				"ManyRouter channel preflight failed for %s: %s",
				desired.ManagedTag,
				common.LocalLogPreview(result.localErr.Error()),
			))
			return fmt.Errorf("ManyRouter channel preflight failed for %s", desired.ManagedTag)
		}
	}
	return nil
}

func manyRouterChannelPreflightMatches(actual dto.ManyRouterManagedChannel, desired dto.ManyRouterDesiredChannel) bool {
	return actual.Name == desired.Name && actual.BaseURL == desired.BaseURL &&
		actual.CredentialVersion == desired.CredentialVersion && slices.Equal(actual.Models, desired.Models) &&
		slices.Equal(actual.Groups, desired.Groups) && actual.Priority == desired.Priority && actual.Weight == desired.Weight
}

func writeManyRouterSyncError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, manyrouterservice.ErrSyncInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": err.Error()})
	case errors.Is(err, model.ErrManyRouterStateConflict), errors.Is(err, model.ErrManyRouterOperationConflict), errors.Is(err, model.ErrManyRouterOwnershipConflict):
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Unable to apply ManyRouter managed state"})
	}
}
