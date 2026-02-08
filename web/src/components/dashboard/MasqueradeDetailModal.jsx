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

import React, { useMemo, useState, useEffect, useRef, useCallback } from 'react';
import { Modal, Tabs, TabPane, Typography, Button, Spin, Empty } from '@douyinfe/semi-ui';
import { IconCopy, IconMaximize, IconMinimize } from '@douyinfe/semi-icons';
import { copy, showError, showSuccess, timestamp2string } from '../../helpers';
import { MasqueradeAPI } from '../../services/masqueradeApi';
import CollapsibleJsonTree from '../common/ui/CollapsibleJsonTree';
import { computeJsonDiff, getDiffColorClass, DIFF_STATUS } from '../../utils/jsonDiff';

const { Text } = Typography;

const safeJsonParse = (value) => {
  if (!value) return null;
  try {
    return JSON.parse(value);
  } catch (e) {
    return null;
  }
};

const safeJsonPrettify = (value) => {
  if (!value) return '';
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch (e) {
    return value;
  }
};

/**
 * MasqueradeDetailModal - Enhanced detail modal with lazy loading, resizable, and sync scrolling
 *
 * @param {Object} props
 * @param {boolean} props.visible - Whether modal is visible
 * @param {string} props.recordId - ID of the record to load (lazy loading)
 * @param {Object} [props.record] - Pre-loaded record data (for backward compatibility)
 * @param {Function} props.onClose - Close callback
 * @param {Function} props.t - Translation function
 */
const MasqueradeDetailModal = ({ visible, recordId, record: preloadedRecord, onClose, t }) => {
  const [activeTab, setActiveTab] = useState('headers');
  const [isMaximized, setIsMaximized] = useState(false);
  const [loading, setLoading] = useState(false);
  const [record, setRecord] = useState(null);

  // Refs for sync scrolling
  const leftScrollRef = useRef(null);
  const rightScrollRef = useRef(null);
  const isScrollingSyncRef = useRef(false);

  // Load record data when modal opens
  useEffect(() => {
    let cancelled = false;

    if (!visible) {
      setRecord(null);
      setLoading(false);
      return;
    }

    // If preloaded record is provided, use it directly
    if (preloadedRecord) {
      setRecord(preloadedRecord);
      return;
    }

    // Otherwise, fetch by ID
    if (recordId) {
      setLoading(true);
      setRecord(null);
      MasqueradeAPI.fetchTraceDetail(recordId)
        .then((data) => {
          if (!cancelled) {
            setRecord(data);
          }
        })
        .catch(() => {
          // Error handled in API
        })
        .finally(() => {
          if (!cancelled) {
            setLoading(false);
          }
        });
    }

    return () => {
      cancelled = true;
    };
  }, [visible, recordId, preloadedRecord]);

  // Compute header diff
  const headerDiff = useMemo(() => {
    if (!record) return { keys: [], diffMap: new Map() };

    const original = record.original_headers || {};
    const masked = record.masked_headers || {};
    const keys = new Set([...Object.keys(original), ...Object.keys(masked)]);
    const sortedKeys = Array.from(keys).sort((a, b) => a.localeCompare(b));

    // Compute diff status for each key
    const diffMap = new Map();
    sortedKeys.forEach((key) => {
      const origVal = original[key];
      const maskedVal = masked[key];

      if (origVal !== undefined && maskedVal === undefined) {
        diffMap.set(key, DIFF_STATUS.REMOVED);
      } else if (origVal === undefined && maskedVal !== undefined) {
        diffMap.set(key, DIFF_STATUS.ADDED);
      } else if (origVal !== maskedVal) {
        diffMap.set(key, DIFF_STATUS.MODIFIED);
      }
    });

    return { keys: sortedKeys, diffMap };
  }, [record]);

  // Parse and compute body diff
  const bodyDiff = useMemo(() => {
    if (!record) return { originalParsed: null, maskedParsed: null, diffMap: new Map() };

    const originalParsed = safeJsonParse(record.original_body);
    const maskedParsed = safeJsonParse(record.masked_body);
    const diffMap = computeJsonDiff(originalParsed, maskedParsed);

    return { originalParsed, maskedParsed, diffMap };
  }, [record]);

  // Sync scroll handler
  const handleLeftScroll = useCallback((e) => {
    if (isScrollingSyncRef.current) return;
    isScrollingSyncRef.current = true;

    const left = e.target;
    const right = rightScrollRef.current;
    if (right) {
      // Calculate scroll ratio
      const scrollRatio = left.scrollTop / (left.scrollHeight - left.clientHeight || 1);
      right.scrollTop = scrollRatio * (right.scrollHeight - right.clientHeight);
    }

    requestAnimationFrame(() => {
      isScrollingSyncRef.current = false;
    });
  }, []);

  const handleRightScroll = useCallback((e) => {
    if (isScrollingSyncRef.current) return;
    isScrollingSyncRef.current = true;

    const right = e.target;
    const left = leftScrollRef.current;
    if (left) {
      const scrollRatio = right.scrollTop / (right.scrollHeight - right.clientHeight || 1);
      left.scrollTop = scrollRatio * (left.scrollHeight - left.clientHeight);
    }

    requestAnimationFrame(() => {
      isScrollingSyncRef.current = false;
    });
  }, []);

  const copyToClipboard = async (text, label) => {
    const ok = await copy(text || '');
    if (ok) {
      showSuccess(`${label} ${t('已复制到剪贴板')}`);
    } else {
      showError(t('复制失败'));
    }
  };

  const toggleMaximize = () => {
    setIsMaximized(!isMaximized);
  };

  // Modal dimensions
  const modalWidth = isMaximized ? '95vw' : 960;
  const bodyMaxHeight = isMaximized ? '90vh' : '70vh';

  // Render header row with diff highlighting
  const renderHeaderRow = (key, value, side, status) => {
    const colorClass = getDiffColorClass(status, side);

    return (
      <div
        key={key}
        className={`px-1 py-0.5 rounded ${colorClass}`}
      >
        <span className='text-blue-600 dark:text-blue-400'>
          {key}
        </span>
        {': '}
        {value !== undefined && value !== '' ? value : '(empty)'}
      </div>
    );
  };

  return (
    <Modal
      title={
        <div className="flex items-center justify-between w-full pr-8">
          <span>{t('伪装对比详情')}</span>
          <Button
            icon={isMaximized ? <IconMinimize /> : <IconMaximize />}
            theme="borderless"
            onClick={toggleMaximize}
            className="ml-4"
          />
        </div>
      }
      visible={visible}
      onCancel={onClose}
      footer={null}
      width={modalWidth}
      centered
      bodyStyle={{
        maxHeight: bodyMaxHeight,
        overflow: 'auto',
        transition: 'all 0.2s ease',
      }}
    >
      {loading ? (
        <div className="flex justify-center items-center py-20">
          <Spin size="large" />
        </div>
      ) : !record ? (
        <Empty
          title={t('记录不存在')}
          description={t('该追踪记录可能已被清除')}
        />
      ) : (
        <>
          {/* Basic info */}
          <div className='mb-4 p-3 bg-gray-100 dark:bg-gray-700 rounded'>
            <div className='grid grid-cols-1 md:grid-cols-4 gap-3 text-sm'>
              <div>
                <Text type='tertiary'>{t('时间')}</Text>
                <div>{record.timestamp ? timestamp2string(record.timestamp) : '-'}</div>
              </div>
              <div>
                <Text type='tertiary'>{t('模型')}</Text>
                <div className='truncate' title={record.model}>
                  {record.model || '-'}
                </div>
              </div>
              <div>
                <Text type='tertiary'>{t('渠道')}</Text>
                <div className='truncate' title={record.channel_name}>
                  {record.channel_name || '-'}{' '}
                  {record.channel_id ? `(ID: ${record.channel_id})` : ''}
                </div>
              </div>
              <div>
                <Text type='tertiary'>{t('Session')}</Text>
                <div className='truncate' title={record.masked_session || ''}>
                  {record.masked_session
                    ? `${record.masked_session.substring(0, 8)}...`
                    : '-'}
                </div>
              </div>
            </div>
          </div>

          {/* Diff legend */}
          <div className="mb-3 flex gap-4 text-xs">
            <span className="flex items-center gap-1">
              <span className="w-3 h-3 bg-yellow-100 dark:bg-yellow-900/40 rounded"></span>
              {t('已修改')}
            </span>
            <span className="flex items-center gap-1">
              <span className="w-3 h-3 bg-red-100 dark:bg-red-900/40 rounded"></span>
              {t('已删除')}
            </span>
            <span className="flex items-center gap-1">
              <span className="w-3 h-3 bg-green-100 dark:bg-green-900/40 rounded"></span>
              {t('新增')}
            </span>
          </div>

          <Tabs activeKey={activeTab} onChange={setActiveTab} type='button'>
            <TabPane tab={t('请求头对比')} itemKey='headers'>
              <div className='grid grid-cols-1 lg:grid-cols-2 gap-4'>
                {/* Original headers */}
                <div className='border rounded p-3 bg-gray-50 dark:bg-gray-800'>
                  <div className='font-semibold mb-2 text-gray-700 dark:text-gray-200'>
                    {t('原始请求')}
                  </div>
                  <div
                    ref={leftScrollRef}
                    onScroll={handleLeftScroll}
                    className='font-mono text-xs overflow-auto max-h-96 space-y-1 whitespace-pre-wrap'
                  >
                    {headerDiff.keys.map((key) => {
                      const status = headerDiff.diffMap.get(key);
                      const value = record.original_headers?.[key];
                      // Don't show added items on left side
                      if (status === DIFF_STATUS.ADDED) return null;
                      return renderHeaderRow(key, value, 'left', status);
                    })}
                  </div>
                  <div className='mt-3 flex gap-2'>
                    <Button
                      size='small'
                      icon={<IconCopy />}
                      onClick={() =>
                        copyToClipboard(
                          JSON.stringify(record.original_headers || {}, null, 2),
                          t('原始请求头'),
                        )
                      }
                    >
                      {t('复制原始')}
                    </Button>
                  </div>
                </div>

                {/* Masked headers */}
                <div className='border rounded p-3 bg-gray-50 dark:bg-gray-800'>
                  <div className='font-semibold mb-2 text-gray-700 dark:text-gray-200'>
                    {t('伪装后请求')}
                  </div>
                  <div
                    ref={rightScrollRef}
                    onScroll={handleRightScroll}
                    className='font-mono text-xs overflow-auto max-h-96 space-y-1 whitespace-pre-wrap'
                  >
                    {headerDiff.keys.map((key) => {
                      const status = headerDiff.diffMap.get(key);
                      const value = record.masked_headers?.[key];
                      // Don't show removed items on right side
                      if (status === DIFF_STATUS.REMOVED) return null;
                      return renderHeaderRow(key, value, 'right', status);
                    })}
                  </div>
                  <div className='mt-3 flex gap-2'>
                    <Button
                      size='small'
                      icon={<IconCopy />}
                      onClick={() =>
                        copyToClipboard(
                          JSON.stringify(record.masked_headers || {}, null, 2),
                          t('伪装请求头'),
                        )
                      }
                    >
                      {t('复制伪装')}
                    </Button>
                  </div>
                </div>
              </div>
            </TabPane>

            <TabPane tab={t('请求体对比')} itemKey='body'>
              <div className='grid grid-cols-1 lg:grid-cols-2 gap-4'>
                {/* Original body */}
                <div className='border rounded p-3 bg-gray-50 dark:bg-gray-800'>
                  <div className='font-semibold mb-2 text-gray-700 dark:text-gray-200'>
                    {t('原始请求')}
                  </div>
                  {bodyDiff.originalParsed !== null ? (
                    <CollapsibleJsonTree
                      data={bodyDiff.originalParsed}
                      diffMap={bodyDiff.diffMap}
                      side="left"
                      defaultExpandDepth={2}
                    />
                  ) : (
                    <pre className='text-xs overflow-auto max-h-96 whitespace-pre-wrap font-mono'>
                      {safeJsonPrettify(record.original_body) || record.original_body || ''}
                    </pre>
                  )}
                  <div className='mt-3 flex gap-2'>
                    <Button
                      size='small'
                      icon={<IconCopy />}
                      onClick={() =>
                        copyToClipboard(record.original_body || '', t('原始请求体'))
                      }
                    >
                      {t('复制原始')}
                    </Button>
                  </div>
                </div>

                {/* Masked body */}
                <div className='border rounded p-3 bg-gray-50 dark:bg-gray-800'>
                  <div className='font-semibold mb-2 text-gray-700 dark:text-gray-200'>
                    {t('伪装后请求')}
                  </div>
                  {bodyDiff.maskedParsed !== null ? (
                    <CollapsibleJsonTree
                      data={bodyDiff.maskedParsed}
                      diffMap={bodyDiff.diffMap}
                      side="right"
                      defaultExpandDepth={2}
                    />
                  ) : (
                    <pre className='text-xs overflow-auto max-h-96 whitespace-pre-wrap font-mono'>
                      {safeJsonPrettify(record.masked_body) || record.masked_body || ''}
                    </pre>
                  )}
                  <div className='mt-3 flex gap-2'>
                    <Button
                      size='small'
                      icon={<IconCopy />}
                      onClick={() =>
                        copyToClipboard(record.masked_body || '', t('伪装请求体'))
                      }
                    >
                      {t('复制伪装')}
                    </Button>
                  </div>
                </div>
              </div>
            </TabPane>
          </Tabs>
        </>
      )}
    </Modal>
  );
};

export default MasqueradeDetailModal;
