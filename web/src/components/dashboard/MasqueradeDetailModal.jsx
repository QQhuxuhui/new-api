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

import React, { useMemo, useState } from 'react';
import { Modal, Tabs, TabPane, Typography, Button } from '@douyinfe/semi-ui';
import { IconCopy } from '@douyinfe/semi-icons';
import { copy, showError, showSuccess, timestamp2string } from '../../helpers';

const { Text } = Typography;

const safeJsonPrettify = (value) => {
  if (!value) return '';
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch (e) {
    return value;
  }
};

const MasqueradeDetailModal = ({ visible, record, onClose, t }) => {
  const [activeTab, setActiveTab] = useState('headers');

  const headerDiffKeys = useMemo(() => {
    const original = record?.original_headers || {};
    const masked = record?.masked_headers || {};
    const keys = new Set([...Object.keys(original), ...Object.keys(masked)]);
    return Array.from(keys).sort((a, b) => a.localeCompare(b));
  }, [record]);

  const originalBodyPretty = useMemo(
    () => safeJsonPrettify(record?.original_body),
    [record],
  );
  const maskedBodyPretty = useMemo(
    () => safeJsonPrettify(record?.masked_body),
    [record],
  );

  const copyToClipboard = async (text, label) => {
    const ok = await copy(text || '');
    if (ok) {
      showSuccess(`${label} ${t('已复制到剪贴板')}`);
    } else {
      showError(t('复制失败'));
    }
  };

  if (!record) return null;

  return (
    <Modal
      title={t('伪装对比详情')}
      visible={visible}
      onCancel={onClose}
      footer={null}
      width={960}
      centered
      bodyStyle={{ maxHeight: '70vh', overflow: 'auto' }}
    >
      {/* 基本信息 */}
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

      <Tabs activeKey={activeTab} onChange={setActiveTab} type='button'>
        <TabPane tab={t('请求头对比')} itemKey='headers'>
          <div className='grid grid-cols-1 lg:grid-cols-2 gap-4'>
            <div className='border rounded p-3 bg-gray-50 dark:bg-gray-800'>
              <div className='font-semibold mb-2 text-gray-700 dark:text-gray-200'>
                {t('原始请求')}
              </div>
              <div className='font-mono text-xs overflow-auto max-h-96 space-y-1 whitespace-pre-wrap'>
                {headerDiffKeys.map((key) => {
                  const original = record.original_headers?.[key];
                  const masked = record.masked_headers?.[key];
                  const isModified =
                    original !== undefined &&
                    masked !== undefined &&
                    original !== masked;
                  const isRemoved =
                    original !== undefined && masked === undefined;

                  return (
                    <div
                      key={key}
                      className={`${isModified ? 'bg-yellow-100 dark:bg-yellow-900/40' : ''} ${isRemoved ? 'bg-red-100 dark:bg-red-900/40' : ''} px-1 rounded`}
                    >
                      <span className='text-blue-600 dark:text-blue-400'>
                        {key}
                      </span>
                      {': '}
                      {original !== undefined && original !== ''
                        ? original
                        : '(empty)'}
                    </div>
                  );
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

            <div className='border rounded p-3 bg-gray-50 dark:bg-gray-800'>
              <div className='font-semibold mb-2 text-gray-700 dark:text-gray-200'>
                {t('伪装后请求')}
              </div>
              <div className='font-mono text-xs overflow-auto max-h-96 space-y-1 whitespace-pre-wrap'>
                {headerDiffKeys.map((key) => {
                  const original = record.original_headers?.[key];
                  const masked = record.masked_headers?.[key];
                  const isModified =
                    original !== undefined &&
                    masked !== undefined &&
                    original !== masked;
                  const isNew =
                    original === undefined && masked !== undefined;

                  return (
                    <div
                      key={key}
                      className={`${isModified ? 'bg-yellow-100 dark:bg-yellow-900/40' : ''} ${isNew ? 'bg-green-100 dark:bg-green-900/40' : ''} px-1 rounded`}
                    >
                      <span className='text-blue-600 dark:text-blue-400'>
                        {key}
                      </span>
                      {': '}
                      {masked !== undefined && masked !== ''
                        ? masked
                        : '(empty)'}
                    </div>
                  );
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
            <div className='border rounded p-3 bg-gray-50 dark:bg-gray-800'>
              <div className='font-semibold mb-2 text-gray-700 dark:text-gray-200'>
                {t('原始请求')}
              </div>
              <pre className='text-xs overflow-auto max-h-96 whitespace-pre-wrap font-mono'>
                {originalBodyPretty || record.original_body || ''}
              </pre>
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

            <div className='border rounded p-3 bg-gray-50 dark:bg-gray-800'>
              <div className='font-semibold mb-2 text-gray-700 dark:text-gray-200'>
                {t('伪装后请求')}
              </div>
              <pre className='text-xs overflow-auto max-h-96 whitespace-pre-wrap font-mono'>
                {maskedBodyPretty || record.masked_body || ''}
              </pre>
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
    </Modal>
  );
};

export default MasqueradeDetailModal;
