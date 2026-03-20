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

export function getLogOther(otherStr) {
  if (otherStr === undefined || otherStr === '') {
    otherStr = '{}';
  }
  let other = JSON.parse(otherStr);
  return other;
}

export function calculateNonCachedPromptTokens(
  promptTokens,
  cacheTokens = 0,
  cacheCreationTokens = 0,
) {
  const totalPromptTokens = parseInt(promptTokens, 10) || 0;
  const cachedReadTokens = parseInt(cacheTokens, 10) || 0;
  const cachedCreationTokens = parseInt(cacheCreationTokens, 10) || 0;
  const nonCachedPromptTokens =
    totalPromptTokens - cachedReadTokens - cachedCreationTokens;
  return nonCachedPromptTokens > 0 ? nonCachedPromptTokens : 0;
}

export function getDisplayPromptTokens(record) {
  const promptTokens = parseInt(record?.prompt_tokens, 10) || 0;
  if (!record) {
    return 0;
  }

  let other = {};
  if (typeof record.other === 'string') {
    other = getLogOther(record.other);
  } else if (record.other && typeof record.other === 'object') {
    other = record.other;
  }

  if (other?.claude) {
    return promptTokens;
  }

  return calculateNonCachedPromptTokens(
    promptTokens,
    other?.cache_tokens,
    other?.cache_creation_tokens,
  );
}
