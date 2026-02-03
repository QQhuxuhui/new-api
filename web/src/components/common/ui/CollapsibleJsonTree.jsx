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

import React, { useState, useCallback, useMemo } from 'react';
import { Button } from '@douyinfe/semi-ui';
import { IconChevronRight, IconChevronDown } from '@douyinfe/semi-icons';
import { getDiffColorClass, getPathDiffStatus } from '../../../utils/jsonDiff';

/**
 * Get value type color class
 */
const getValueColorClass = (value) => {
  if (value === null) return 'text-gray-400';
  if (typeof value === 'string') return 'text-green-600 dark:text-green-400';
  if (typeof value === 'number') return 'text-blue-600 dark:text-blue-400';
  if (typeof value === 'boolean') return 'text-purple-600 dark:text-purple-400';
  return 'text-gray-700 dark:text-gray-300';
};

/**
 * Format value for display
 */
const formatValue = (value) => {
  if (value === null) return 'null';
  if (typeof value === 'string') return `"${value}"`;
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  return String(value);
};

/**
 * Check if value is expandable (object or array)
 */
const isExpandable = (value) => {
  return value !== null && typeof value === 'object';
};

/**
 * Count items in object or array
 */
const countItems = (value) => {
  if (Array.isArray(value)) return value.length;
  if (value !== null && typeof value === 'object') return Object.keys(value).length;
  return 0;
};

/**
 * Build a set of all paths that should be expanded by default
 */
const buildDefaultExpandedPaths = (data, maxDepth = 2, path = '', depth = 0) => {
  const paths = new Set();

  if (depth >= maxDepth) return paths;
  if (!isExpandable(data)) return paths;

  paths.add(path);

  if (Array.isArray(data)) {
    data.forEach((item, idx) => {
      const childPath = path ? `${path}[${idx}]` : `[${idx}]`;
      const childPaths = buildDefaultExpandedPaths(item, maxDepth, childPath, depth + 1);
      childPaths.forEach((p) => paths.add(p));
    });
  } else if (data !== null && typeof data === 'object') {
    Object.keys(data).forEach((key) => {
      const childPath = path ? `${path}.${key}` : key;
      const childPaths = buildDefaultExpandedPaths(data[key], maxDepth, childPath, depth + 1);
      childPaths.forEach((p) => paths.add(p));
    });
  }

  return paths;
};

/**
 * Build a set of all expandable paths in the data
 */
const buildAllExpandablePaths = (data, path = '') => {
  const paths = new Set();

  if (!isExpandable(data)) return paths;

  paths.add(path);

  if (Array.isArray(data)) {
    data.forEach((item, idx) => {
      const childPath = path ? `${path}[${idx}]` : `[${idx}]`;
      const childPaths = buildAllExpandablePaths(item, childPath);
      childPaths.forEach((p) => paths.add(p));
    });
  } else if (data !== null && typeof data === 'object') {
    Object.keys(data).forEach((key) => {
      const childPath = path ? `${path}.${key}` : key;
      const childPaths = buildAllExpandablePaths(data[key], childPath);
      childPaths.forEach((p) => paths.add(p));
    });
  }

  return paths;
};

/**
 * JSON Node component for rendering a single key-value pair
 */
const JsonNode = ({
  keyName,
  value,
  path,
  depth,
  expanded,
  onToggle,
  diffMap,
  side,
}) => {
  const isExp = isExpandable(value);
  const isExpanded = expanded.has(path);
  const itemCount = countItems(value);

  // Get diff status for this path
  const diffStatus = diffMap ? getPathDiffStatus(diffMap, path) : null;
  const diffColorClass = getDiffColorClass(diffStatus, side);

  const handleToggle = useCallback(() => {
    if (isExp) {
      onToggle(path);
    }
  }, [isExp, path, onToggle]);

  const indent = depth * 16;

  return (
    <div className="select-text">
      {/* Current node line */}
      <div
        className={`flex items-start py-0.5 px-1 rounded ${diffColorClass}`}
        style={{ paddingLeft: `${indent}px` }}
      >
        {/* Expand/collapse button */}
        {isExp ? (
          <button
            onClick={handleToggle}
            className="flex-shrink-0 w-4 h-4 mr-1 flex items-center justify-center text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
          >
            {isExpanded ? (
              <IconChevronDown size="small" />
            ) : (
              <IconChevronRight size="small" />
            )}
          </button>
        ) : (
          <span className="w-4 mr-1 flex-shrink-0" />
        )}

        {/* Key name */}
        {keyName !== null && (
          <>
            <span className="text-blue-600 dark:text-blue-400 flex-shrink-0">
              {typeof keyName === 'number' ? `[${keyName}]` : `"${keyName}"`}
            </span>
            <span className="text-gray-600 dark:text-gray-400 mx-1 flex-shrink-0">:</span>
          </>
        )}

        {/* Value */}
        {isExp ? (
          <span className="text-gray-500">
            {Array.isArray(value) ? `Array(${itemCount})` : `Object{${itemCount}}`}
            {!isExpanded && <span className="ml-1 text-gray-400">...</span>}
          </span>
        ) : (
          <span className={`${getValueColorClass(value)} break-all`}>
            {formatValue(value)}
          </span>
        )}
      </div>

      {/* Children (if expanded) */}
      {isExp && isExpanded && (
        <div>
          {Array.isArray(value)
            ? value.map((item, idx) => (
                <JsonNode
                  key={idx}
                  keyName={idx}
                  value={item}
                  path={path ? `${path}[${idx}]` : `[${idx}]`}
                  depth={depth + 1}
                  expanded={expanded}
                  onToggle={onToggle}
                  diffMap={diffMap}
                  side={side}
                />
              ))
            : Object.keys(value).map((key) => (
                <JsonNode
                  key={key}
                  keyName={key}
                  value={value[key]}
                  path={path ? `${path}.${key}` : key}
                  depth={depth + 1}
                  expanded={expanded}
                  onToggle={onToggle}
                  diffMap={diffMap}
                  side={side}
                />
              ))}
        </div>
      )}
    </div>
  );
};

/**
 * CollapsibleJsonTree - A component for rendering JSON with collapsible nodes
 *
 * @param {Object} props
 * @param {any} props.data - JSON data to render
 * @param {Map<string, string>} [props.diffMap] - Optional diff map from computeJsonDiff
 * @param {'left' | 'right'} [props.side] - Which side this tree represents (for diff coloring)
 * @param {number} [props.defaultExpandDepth=2] - Default expansion depth
 * @param {Function} [props.onScroll] - Scroll event handler
 * @param {React.Ref} [props.scrollRef] - Ref for the scroll container
 */
const CollapsibleJsonTree = ({
  data,
  diffMap,
  side,
  defaultExpandDepth = 2,
  onScroll,
  scrollRef,
}) => {
  // Initialize expanded paths with default depth
  const defaultExpanded = useMemo(() => {
    return buildDefaultExpandedPaths(data, defaultExpandDepth);
  }, [data, defaultExpandDepth]);

  const [expanded, setExpanded] = useState(defaultExpanded);

  // All expandable paths for "expand all" feature
  const allExpandable = useMemo(() => {
    return buildAllExpandablePaths(data);
  }, [data]);

  const handleToggle = useCallback((path) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(path)) {
        next.delete(path);
      } else {
        next.add(path);
      }
      return next;
    });
  }, []);

  const handleExpandAll = useCallback(() => {
    setExpanded(new Set(allExpandable));
  }, [allExpandable]);

  const handleCollapseAll = useCallback(() => {
    setExpanded(new Set());
  }, []);

  if (data === undefined || data === null) {
    return (
      <div className="text-gray-400 italic p-2">
        (empty)
      </div>
    );
  }

  return (
    <div className="font-mono text-xs">
      {/* Control buttons */}
      <div className="flex gap-2 mb-2 sticky top-0 bg-gray-50 dark:bg-gray-800 z-10 py-1">
        <Button size="small" onClick={handleExpandAll}>
          展开全部
        </Button>
        <Button size="small" onClick={handleCollapseAll}>
          折叠全部
        </Button>
      </div>

      {/* JSON tree */}
      <div
        ref={scrollRef}
        onScroll={onScroll}
        className="overflow-auto max-h-96"
      >
        {isExpandable(data) ? (
          <JsonNode
            keyName={null}
            value={data}
            path=""
            depth={0}
            expanded={expanded}
            onToggle={handleToggle}
            diffMap={diffMap}
            side={side}
          />
        ) : (
          <span className={getValueColorClass(data)}>
            {formatValue(data)}
          </span>
        )}
      </div>
    </div>
  );
};

export default CollapsibleJsonTree;
