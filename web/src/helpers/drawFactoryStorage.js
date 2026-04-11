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

// Single namespace module for DrawFactory localStorage.
// All reads/writes go through here so future schema migrations have a single
// anchor.

export const KEYS = {
  history: 'drawFactory.history.v1',
  batchJobs: 'drawFactory.batchJobs.v1',
  lastConfig: 'drawFactory.lastConfig.v1',
};

export const HISTORY_MAX = 50;

function safeGet(key, fallback) {
  try {
    const raw = window.localStorage.getItem(key);
    if (!raw) return fallback;
    return JSON.parse(raw);
  } catch (_e) {
    return fallback;
  }
}

function safeSet(key, value) {
  const payload = JSON.stringify(value);
  try {
    window.localStorage.setItem(key, payload);
    return { ok: true };
  } catch (e) {
    // QuotaExceeded: trim history by half and retry once.
    if (key === KEYS.history && Array.isArray(value) && value.length > 0) {
      const trimmed = value.slice(Math.floor(value.length / 2));
      try {
        window.localStorage.setItem(key, JSON.stringify(trimmed));
        return { ok: true, trimmed: true };
      } catch (_e2) {
        return { ok: false, error: 'quota' };
      }
    }
    return { ok: false, error: 'quota' };
  }
}

export function getHistory() {
  const list = safeGet(KEYS.history, []);
  return Array.isArray(list) ? list : [];
}

export function addHistory(entry) {
  const list = getHistory();
  list.unshift(entry);
  const capped = list.slice(0, HISTORY_MAX);
  return safeSet(KEYS.history, capped);
}

export function clearHistory() {
  window.localStorage.removeItem(KEYS.history);
}

export function getBatchJobs() {
  const jobs = safeGet(KEYS.batchJobs, []);
  return Array.isArray(jobs) ? jobs : [];
}

export function saveBatchJobs(jobs) {
  return safeSet(KEYS.batchJobs, jobs);
}

export function clearBatchJobs() {
  window.localStorage.removeItem(KEYS.batchJobs);
}

export function getLastConfig() {
  return safeGet(KEYS.lastConfig, null);
}

export function saveLastConfig(config) {
  return safeSet(KEYS.lastConfig, config);
}
