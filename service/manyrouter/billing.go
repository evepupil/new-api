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
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

var syncBillingOptionKeys = map[string]bool{
	"ModelRatio": true, "CompletionRatio": true, "ModelPrice": true,
	"CacheRatio": true, "CreateCacheRatio": true, "CreateCacheRatio5m": true,
	"CreateCacheRatio1h": true, "CacheCreationRatio": true, "ImageRatio": true,
	"AudioRatio": true, "AudioCompletionRatio": true, "GroupGroupRatio": true,
	"QuotaPerUnit": true, "USDExchangeRate": true, "UnitPrice": true,
}

func SyncBillingBasis() (map[string]json.RawMessage, string, error) {
	common.OptionMapRWMutex.RLock()
	options := make(map[string]string, len(syncBillingOptionKeys))
	for key := range syncBillingOptionKeys {
		if value, exists := common.OptionMap[key]; exists {
			options[key] = value
		}
	}
	common.OptionMapRWMutex.RUnlock()

	basis := make(map[string]json.RawMessage, len(options)+1)
	version, err := common.Marshal(common.Version)
	if err != nil {
		return nil, "", err
	}
	basis["NewAPIVersion"] = version
	for key, raw := range options {
		var value any
		if err := common.DecodeJsonUseNumber(strings.NewReader(raw), &value); err != nil {
			return nil, "", err
		}
		normalized, err := normalizeSyncBillingValue(value)
		if err != nil {
			return nil, "", err
		}
		encoded, err := common.Marshal(normalized)
		if err != nil {
			return nil, "", err
		}
		basis[key] = encoded
	}
	for _, required := range []string{"ModelRatio", "CompletionRatio", "ModelPrice"} {
		if _, exists := basis[required]; !exists {
			return nil, "", errors.New("required pricing settings are unavailable")
		}
	}
	encoded, err := common.Marshal(basis)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(encoded)
	return basis, hex.EncodeToString(digest[:]), nil
}

func normalizeSyncBillingValue(value any) (any, error) {
	switch typed := value.(type) {
	case json.Number:
		number, err := decimal.NewFromString(string(typed))
		if err != nil {
			return nil, err
		}
		return json.Number(number.String()), nil
	case map[string]any:
		for key, item := range typed {
			normalized, err := normalizeSyncBillingValue(item)
			if err != nil {
				return nil, err
			}
			typed[key] = normalized
		}
	case []any:
		for index, item := range typed {
			normalized, err := normalizeSyncBillingValue(item)
			if err != nil {
				return nil, err
			}
			typed[index] = normalized
		}
	}
	return value, nil
}
