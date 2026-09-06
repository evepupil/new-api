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
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetManyRouterSyncLogs(c *gin.Context) {
	page := common.GetPageQuery(c)
	logType, err := strconv.Atoi(strings.TrimSpace(c.Query("type")))
	if err != nil || (logType != model.LogTypeConsume && logType != model.LogTypeError) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "ManyRouter log type is invalid"})
		return
	}
	start, ok := manyRouterLogTimestamp(c, "start_timestamp")
	if !ok {
		return
	}
	end, ok := manyRouterLogTimestamp(c, "end_timestamp")
	if !ok {
		return
	}
	if end > 0 && start > end {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "ManyRouter log time range is invalid"})
		return
	}
	logs, total, err := model.GetAllLogs(
		logType, start, end, "", "", "", page.GetStartIdx(), page.GetPageSize(), 0, "", "", "",
	)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": "ManyRouter logs are unavailable"})
		return
	}
	items := make([]dto.ManyRouterSyncLog, 0, len(logs))
	for _, entry := range logs {
		items = append(items, dto.ManyRouterSyncLog{
			ID: int64(entry.Id), CreatedAt: entry.CreatedAt, Type: entry.Type, Content: entry.Content,
			Model: entry.ModelName, InputTokens: int64(entry.PromptTokens), OutputTokens: int64(entry.CompletionTokens),
			DurationSeconds: int64(entry.UseTime), Stream: entry.IsStream, ChannelID: int64(entry.ChannelId),
			Group: entry.Group, RequestID: entry.RequestId, UpstreamRequestID: entry.UpstreamRequestId, Other: entry.Other,
		})
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": dto.ManyRouterSyncLogPage{
		Items: items, Total: total, Page: page.GetPage(), PageSize: page.GetPageSize(),
	}})
}

func manyRouterLogTimestamp(c *gin.Context, name string) (int64, bool) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return 0, true
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "ManyRouter log timestamp is invalid"})
		return 0, false
	}
	return value, true
}
