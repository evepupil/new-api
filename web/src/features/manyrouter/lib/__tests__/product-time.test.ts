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
import { describe, expect, test } from 'vitest'

import { formatProductTime } from '../product-time'

describe('ManyRouter product time formatting', () => {
  test('maps the project zhCN language before calling the browser Intl API', () => {
    const timestamp = '2026-09-06T15:00:00Z'

    expect(() => formatProductTime(timestamp, 'zhCN')).not.toThrow()
    expect(formatProductTime(timestamp, 'zhCN')).toBe(
      new Date(timestamp).toLocaleString('zh-CN')
    )
  })
})
