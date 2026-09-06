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

package manyrouter_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/service/manyrouter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProductsUsesServerTokenAndValidatesContract(t *testing.T) {
	t.Parallel()
	token := "mrp_" + strings.Repeat("a", 43)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/api/v1/site/products", request.URL.Path)
		assert.Equal(t, "Bearer "+token, request.Header.Get("Authorization"))
		response.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(response, `{
			"id":"snapshot","contract_version":"m3-site-products-v1","version":1,
			"site_id":"site","site_name":"Site","route_plan_id":"plan",
			"generated_at":%q,"content_hash":%q,"products":[]
		}`, time.Now().UTC().Format(time.RFC3339Nano), strings.Repeat("b", 64))
	}))
	defer server.Close()
	client, err := manyrouter.New(server.URL, token, server.Client())
	require.NoError(t, err)
	snapshot, err := client.Products(context.Background())
	require.NoError(t, err)
	assert.Equal(t, manyrouter.ContractVersion, snapshot.ContractVersion)
	assert.Equal(t, int64(1), snapshot.Version)
}

func TestProductsRejectsBadConfigurationAndUpstreamErrors(t *testing.T) {
	t.Parallel()
	_, err := manyrouter.New("file:///tmp/data", "bad", nil)
	require.Error(t, err)
	token := "mrp_" + strings.Repeat("a", 43)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, "private upstream detail", http.StatusBadGateway)
	}))
	defer server.Close()
	client, err := manyrouter.New(server.URL, token, server.Client())
	require.NoError(t, err)
	_, err = client.Products(context.Background())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "private upstream detail")
}
