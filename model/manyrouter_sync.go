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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrManyRouterStateConflict     = errors.New("manyrouter managed state changed")
	ErrManyRouterOperationConflict = errors.New("manyrouter operation ID was reused")
	ErrManyRouterOwnershipConflict = errors.New("manyrouter managed resource ownership conflict")
)

const manyRouterReceiptTTL = 24 * time.Hour

type ManyRouterSyncReceipt struct {
	OperationID string `gorm:"primaryKey;size:128"`
	RequestHash string `gorm:"size:64;not null"`
	Response    []byte `gorm:"size:2097152;not null"`
	CreatedAt   int64  `gorm:"not null;index"`
}

func ReadManyRouterManagedState() (dto.ManyRouterManagedState, error) {
	if DB == nil {
		return dto.ManyRouterManagedState{}, errors.New("database is unavailable")
	}
	return readManyRouterManagedState(DB, false)
}

func ApplyManyRouterManagedState(
	request dto.ManyRouterApplyRequest,
	requestHash string,
	now time.Time,
) (dto.ManyRouterApplyResponse, error) {
	if DB == nil {
		return dto.ManyRouterApplyResponse{}, errors.New("database is unavailable")
	}
	now = now.UTC()
	var response dto.ManyRouterApplyResponse
	var optionValues map[string]string
	replayed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureManyRouterOptions(tx); err != nil {
			return err
		}
		var lockedOptions []Option
		if err := lockForUpdate(tx).
			Where(map[string]any{"key": []string{"GroupRatio", "UserUsableGroups"}}).
			Order(clause.OrderByColumn{Column: clause.Column{Name: "key"}}).
			Find(&lockedOptions).Error; err != nil {
			return err
		}

		var receipt ManyRouterSyncReceipt
		receiptQuery := lockForUpdate(tx).Where("operation_id = ?", request.OperationID).Limit(1).Find(&receipt)
		if receiptQuery.Error != nil {
			return receiptQuery.Error
		}
		if receiptQuery.RowsAffected > 0 {
			if receipt.RequestHash != requestHash {
				return ErrManyRouterOperationConflict
			}
			if err := common.Unmarshal(receipt.Response, &response); err != nil {
				return err
			}
			response.Replayed = true
			replayed = true
			return nil
		}

		current, err := readManyRouterManagedState(tx, true)
		if err != nil {
			return err
		}
		if current.StateHash != request.ExpectedStateHash {
			return ErrManyRouterStateConflict
		}
		if len(current.Conflicts) > 0 {
			return fmt.Errorf("%w: %s", ErrManyRouterOwnershipConflict, strings.Join(current.Conflicts, "; "))
		}

		actions, err := applyManyRouterChannels(tx, request.Channels)
		if err != nil {
			return err
		}
		groupActions, values, err := applyManyRouterGroups(tx, request.Groups)
		if err != nil {
			return err
		}
		actions = append(actions, groupActions...)
		optionValues = values

		state, err := readManyRouterManagedState(tx, false)
		if err != nil {
			return err
		}
		response = dto.ManyRouterApplyResponse{
			OperationID:      request.OperationID,
			RoutePlanVersion: request.RoutePlanVersion,
			Actions:          actions,
			State:            state,
		}
		encoded, err := common.Marshal(response)
		if err != nil {
			return err
		}
		if err := tx.Create(&ManyRouterSyncReceipt{
			OperationID: request.OperationID,
			RequestHash: requestHash,
			Response:    append([]byte(nil), encoded...),
			CreatedAt:   now.Unix(),
		}).Error; err != nil {
			return err
		}
		return tx.Where("created_at < ?", now.Add(-manyRouterReceiptTTL).Unix()).
			Delete(&ManyRouterSyncReceipt{}).Error
	})
	if err != nil || replayed {
		return response, err
	}
	for key, value := range optionValues {
		if err := updateOptionMap(key, value); err != nil {
			return dto.ManyRouterApplyResponse{}, err
		}
	}
	InitChannelCache()
	return response, nil
}
