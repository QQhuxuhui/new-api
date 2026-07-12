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

import assert from 'node:assert/strict';
import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import test from 'node:test';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const componentDir = join(here, 'components');
const sourceFiles = [
  join(here, 'index.jsx'),
  ...readdirSync(componentDir)
    .filter((name) => name.endsWith('.jsx'))
    .map((name) => join(componentDir, name)),
];
const source = sourceFiles.map((file) => readFileSync(file, 'utf8')).join('\n');
const literalKeys = [...source.matchAll(/\bt\(\s*(['"])(.*?)\1/g)].map(
  (match) => match[2],
);
const computedKeys = [
  '已开启自动切换',
  '已关闭自动切换',
  '订阅套餐',
  '按量付费',
  '试用套餐',
  '企业套餐',
  '未知类型',
  '已过期',
  '已停用',
  '已用完',
  '已作废',
  '已回收',
];
const compatibilityKeys = [
  '已切换到该套餐。系统不会自动更换你的选择;额度用尽或渠道故障时仍会自动处理。',
];
const requiredKeys = [
  ...new Set([...literalKeys, ...computedKeys, ...compatibilityKeys]),
].sort();

// These Japanese labels intentionally use the same Han characters (or clock
// notation) as the Chinese source key; equality does not indicate fallback.
const sameAsSourceExemptions = {
  ja: new Set(['手动指定', '解除', '未设置', '明日 00:00']),
};

const loadTranslation = (locale) => {
  const file = join(here, '..', '..', 'i18n', 'locales', `${locale}.json`);
  return JSON.parse(readFileSync(file, 'utf8')).translation;
};

test('every MyPlans key has a non-empty value in every runtime locale', () => {
  const failures = [];
  for (const locale of ['zh', 'en', 'fr', 'ja', 'ru']) {
    const translation = loadTranslation(locale);
    const missing = requiredKeys.filter(
      (key) =>
        typeof translation[key] !== 'string' || translation[key].trim() === '',
    );
    if (missing.length) failures.push(`${locale}: ${missing.join(', ')}`);
  }
  assert.deepEqual(failures, []);
});

test('non-Chinese MyPlans values do not silently fall back to source keys', () => {
  const failures = [];
  for (const locale of ['en', 'fr', 'ja', 'ru']) {
    const translation = loadTranslation(locale);
    const exemptions = sameAsSourceExemptions[locale] ?? new Set();
    const untranslated = requiredKeys.filter(
      (key) => translation[key]?.trim() === key && !exemptions.has(key),
    );
    if (untranslated.length) {
      failures.push(`${locale}: ${untranslated.join(', ')}`);
    }
  }
  assert.deepEqual(failures, []);
});
