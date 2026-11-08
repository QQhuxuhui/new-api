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
import type { TFunction } from 'i18next'

import type { ErrorRuleMatchType } from './types'

export function matchTypeLabel(
  t: TFunction,
  matchType: ErrorRuleMatchType
): string {
  switch (matchType) {
    case 'AND':
      return t('Status code and keyword')
    case 'OR':
      return t('Status code or keyword')
    case 'STATUS_ONLY':
      return t('Status code only')
    case 'KEYWORD_ONLY':
      return t('Keyword only')
  }
}
