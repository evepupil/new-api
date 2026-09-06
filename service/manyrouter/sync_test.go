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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelationIDFromManagedTagRequiresCanonicalUUID(t *testing.T) {
	canonical := "manyrouter:aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	parsed, err := relationIDFromManagedTag(canonical)
	require.NoError(t, err)
	assert.Equal(t, canonical, "manyrouter:"+parsed.String())

	_, err = relationIDFromManagedTag("manyrouter:AAAAAAAA-AAAA-AAAA-AAAA-AAAAAAAAAAAA")
	assert.Error(t, err)
}
