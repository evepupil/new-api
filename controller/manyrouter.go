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

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	manyrouterservice "github.com/QuantumNous/new-api/service/manyrouter"
	"github.com/gin-gonic/gin"
)

func GetManyRouterProducts(c *gin.Context) {
	client, err := manyrouterservice.NewFromEnvironment()
	if err != nil {
		writeManyRouterUnavailable(c)
		return
	}
	snapshot, err := client.Products(c.Request.Context())
	if err != nil {
		writeManyRouterUnavailable(c)
		return
	}
	userID := c.GetInt("id")
	user, err := model.GetUserCache(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "用户登录状态无效"})
		return
	}
	usable := service.GetUserUsableGroups(user.Group)
	snapshot = filterManyRouterProducts(snapshot, usable)
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, gin.H{"success": true, "data": snapshot})
}

func filterManyRouterProducts(snapshot manyrouterservice.Snapshot, usable map[string]string) manyrouterservice.Snapshot {
	products := make([]manyrouterservice.Product, 0, len(snapshot.Products))
	for _, product := range snapshot.Products {
		if !product.EntryOpen {
			continue
		}
		if _, allowed := usable[product.GroupKey]; !allowed {
			continue
		}
		products = append(products, product)
	}
	snapshot.Products = products
	return snapshot
}

func writeManyRouterUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{
		"success": false,
		"message": "模型与 Auto 数据暂时无法更新，请稍后重试",
	})
}
