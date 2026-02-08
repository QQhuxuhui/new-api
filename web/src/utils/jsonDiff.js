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

/**
 * Diff status constants
 */
export const DIFF_STATUS = {
  UNCHANGED: 'unchanged',
  MODIFIED: 'modified',
  ADDED: 'added',
  REMOVED: 'removed',
};

/**
 * Check if value is a plain object
 */
const isPlainObject = (val) => {
  return val !== null && typeof val === 'object' && !Array.isArray(val);
};

/**
 * Deep equality check for JSON values
 */
const deepEqual = (a, b) => {
  if (a === b) return true;
  if (typeof a !== typeof b) return false;
  if (a === null || b === null) return a === b;

  if (Array.isArray(a) && Array.isArray(b)) {
    if (a.length !== b.length) return false;
    return a.every((val, idx) => deepEqual(val, b[idx]));
  }

  if (isPlainObject(a) && isPlainObject(b)) {
    const keysA = Object.keys(a);
    const keysB = Object.keys(b);
    if (keysA.length !== keysB.length) return false;
    return keysA.every((key) => deepEqual(a[key], b[key]));
  }

  return false;
};

/**
 * Recursively compute diff between two JSON objects
 * @param {any} original - Original JSON value
 * @param {any} masked - Masked/modified JSON value
 * @param {string} path - Current path prefix (e.g., 'metadata.user_id')
 * @returns {Map<string, string>} - Map of path -> DIFF_STATUS
 */
export const computeJsonDiff = (original, masked, path = '') => {
  const diffMap = new Map();

  // Both null/undefined
  if (original == null && masked == null) {
    return diffMap;
  }

  // Added (original is null/undefined, masked has value)
  if (original == null && masked != null) {
    diffMap.set(path || '.', DIFF_STATUS.ADDED);
    // If masked is object/array, mark all children as added
    if (isPlainObject(masked)) {
      Object.keys(masked).forEach((key) => {
        const childPath = path ? `${path}.${key}` : key;
        const childDiff = computeJsonDiff(null, masked[key], childPath);
        childDiff.forEach((status, p) => diffMap.set(p, status));
      });
    } else if (Array.isArray(masked)) {
      masked.forEach((item, idx) => {
        const childPath = path ? `${path}[${idx}]` : `[${idx}]`;
        const childDiff = computeJsonDiff(null, item, childPath);
        childDiff.forEach((status, p) => diffMap.set(p, status));
      });
    }
    return diffMap;
  }

  // Removed (original has value, masked is null/undefined)
  if (original != null && masked == null) {
    diffMap.set(path || '.', DIFF_STATUS.REMOVED);
    // If original is object/array, mark all children as removed
    if (isPlainObject(original)) {
      Object.keys(original).forEach((key) => {
        const childPath = path ? `${path}.${key}` : key;
        const childDiff = computeJsonDiff(original[key], null, childPath);
        childDiff.forEach((status, p) => diffMap.set(p, status));
      });
    } else if (Array.isArray(original)) {
      original.forEach((item, idx) => {
        const childPath = path ? `${path}[${idx}]` : `[${idx}]`;
        const childDiff = computeJsonDiff(item, null, childPath);
        childDiff.forEach((status, p) => diffMap.set(p, status));
      });
    }
    return diffMap;
  }

  // Both are objects
  if (isPlainObject(original) && isPlainObject(masked)) {
    const allKeys = new Set([
      ...Object.keys(original),
      ...Object.keys(masked),
    ]);

    allKeys.forEach((key) => {
      const childPath = path ? `${path}.${key}` : key;
      const childDiff = computeJsonDiff(original[key], masked[key], childPath);
      childDiff.forEach((status, p) => diffMap.set(p, status));
    });

    return diffMap;
  }

  // Both are arrays
  if (Array.isArray(original) && Array.isArray(masked)) {
    const maxLen = Math.max(original.length, masked.length);

    for (let i = 0; i < maxLen; i++) {
      const childPath = path ? `${path}[${i}]` : `[${i}]`;
      const origItem = i < original.length ? original[i] : undefined;
      const maskedItem = i < masked.length ? masked[i] : undefined;
      const childDiff = computeJsonDiff(origItem, maskedItem, childPath);
      childDiff.forEach((status, p) => diffMap.set(p, status));
    }

    return diffMap;
  }

  // Primitive values comparison
  if (deepEqual(original, masked)) {
    // Unchanged, no entry needed
  } else {
    diffMap.set(path || '.', DIFF_STATUS.MODIFIED);
  }

  return diffMap;
};

/**
 * Get CSS class name for diff highlighting based on status and side
 * @param {string} status - One of DIFF_STATUS values
 * @param {'left' | 'right'} side - Which side (left=original, right=masked)
 * @returns {string} - Tailwind CSS class names
 */
export const getDiffColorClass = (status, side) => {
  if (!status || status === DIFF_STATUS.UNCHANGED) {
    return '';
  }

  switch (status) {
    case DIFF_STATUS.MODIFIED:
      return 'bg-yellow-100 dark:bg-yellow-900/40';
    case DIFF_STATUS.ADDED:
      return side === 'right' ? 'bg-green-100 dark:bg-green-900/40' : '';
    case DIFF_STATUS.REMOVED:
      return side === 'left' ? 'bg-red-100 dark:bg-red-900/40' : '';
    default:
      return '';
  }
};

/**
 * Check if a path or any of its parent/child paths have a diff status
 * @param {Map<string, string>} diffMap - The diff map
 * @param {string} path - Path to check
 * @returns {string | null} - The diff status if found, null otherwise
 */
export const getPathDiffStatus = (diffMap, path) => {
  const normalizedPath =
    path === '' ? '.' : path === '.' ? '' : path;

  // Direct match
  if (diffMap.has(path)) {
    return diffMap.get(path);
  }
  if (diffMap.has(normalizedPath)) {
    return diffMap.get(normalizedPath);
  }

  // Check if any parent path has a status (for inherited status)
  const parts = path.split('.');
  for (let i = parts.length - 1; i > 0; i--) {
    const parentPath = parts.slice(0, i).join('.');
    if (diffMap.has(parentPath)) {
      return diffMap.get(parentPath);
    }
    // Handle root path normalization
    if (parentPath === '' && diffMap.has('.')) {
      return diffMap.get('.');
    }
  }

  return null;
};
