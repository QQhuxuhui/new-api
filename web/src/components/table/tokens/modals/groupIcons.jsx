/*
Copyright (C) 2025 QuantumNous

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

import React from 'react';
import { IconKey } from '@douyinfe/semi-icons';
import {
  Claude,
  OpenAI,
  Gemini,
  DeepSeek,
  Grok,
  Qwen,
  Mistral,
  Kimi,
  Zhipu,
  Doubao,
} from '@lobehub/icons';

// Default icon size (24px matches Semi UI's extra-large)
export const GROUP_ICON_SIZE = 24;

// Brand-icon detection by group-name keywords, with a generic fallback.
// First match wins, so list more specific keywords first.
// To support a new brand, just add an entry here.
const BRAND_ICONS = [
  {
    keywords: ['claude', 'anthropic'],
    render: (s) => <Claude.Color size={s} />,
  },
  {
    keywords: ['codex', 'openai', 'gpt', 'o1', 'o3'],
    render: (s) => <OpenAI size={s} />,
  },
  { keywords: ['gemini', 'google'], render: (s) => <Gemini.Color size={s} /> },
  { keywords: ['deepseek'], render: (s) => <DeepSeek.Color size={s} /> },
  { keywords: ['grok', 'xai'], render: (s) => <Grok size={s} /> },
  {
    keywords: ['qwen', 'tongyi', '通义', '千问'],
    render: (s) => <Qwen.Color size={s} />,
  },
  { keywords: ['mistral'], render: (s) => <Mistral.Color size={s} /> },
  {
    keywords: ['kimi', 'moonshot', '月之暗面'],
    render: (s) => <Kimi.Color size={s} />,
  },
  {
    keywords: ['glm', 'zhipu', '智谱'],
    render: (s) => <Zhipu.Color size={s} />,
  },
  { keywords: ['doubao', '豆包'], render: (s) => <Doubao.Color size={s} /> },
];

/**
 * Resolve an icon for a group by matching brand keywords in its name,
 * falling back to a generic key icon when nothing matches.
 * @param {string} groupName
 * @param {number} [size=GROUP_ICON_SIZE]
 * @returns {JSX.Element}
 */
export const getGroupIcon = (groupName, size = GROUP_ICON_SIZE) => {
  const name = (groupName || '').toLowerCase();
  for (const brand of BRAND_ICONS) {
    if (brand.keywords.some((k) => name.includes(k.toLowerCase()))) {
      return brand.render(size);
    }
  }
  return <IconKey size='extra-large' />;
};
