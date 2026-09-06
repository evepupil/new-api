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

package middleware

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

func ManyRouterSyncAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		current := strings.TrimSpace(os.Getenv("MANYROUTER_SYNC_TOKEN"))
		previous := strings.TrimSpace(os.Getenv("MANYROUTER_SYNC_TOKEN_PREVIOUS"))
		if len(current) < 32 {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
				"success": false,
				"message": "ManyRouter managed sync is not configured",
			})
			return
		}
		provided := manyRouterBearerToken(c.GetHeader("Authorization"))
		if !manyRouterTokenEqual(provided, current) &&
			(len(previous) < 32 || !manyRouterTokenEqual(provided, previous)) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "ManyRouter managed sync token is invalid",
			})
			return
		}
		c.Header("Cache-Control", "private, no-store")
		c.Next()
	}
}

func manyRouterBearerToken(header string) string {
	scheme, token, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func manyRouterTokenEqual(provided, expected string) bool {
	return len(provided) == len(expected) &&
		subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}
