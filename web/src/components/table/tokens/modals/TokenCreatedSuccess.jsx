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
import {
  Modal,
  Button,
  Typography,
  Toast,
} from '@douyinfe/semi-ui';
import {
  IconTickCircle,
  IconCopy,
} from '@douyinfe/semi-icons';
import { copy } from '../../../../helpers/utils';
import { TokenAnalytics } from '../../../../helpers/analytics';

const { Title, Text } = Typography;

const TokenCreatedSuccess = ({ visible, tokenData, onClose, t }) => {
  if (!tokenData) return null;

  const tokenKey = tokenData.key ? `sk-${tokenData.key}` : '';

  // Get baseURL from system configuration, fallback to window origin
  let baseURL = '';
  try {
    const status = localStorage.getItem('status');
    if (status) {
      const statusObj = JSON.parse(status);
      baseURL = statusObj.server_address || '';
    }
  } catch (e) {
    console.error('Failed to parse status from localStorage:', e);
  }
  if (!baseURL && typeof window !== 'undefined') {
    baseURL = window.location.origin;
  }

  const handleCopy = async (text, message, isTokenKey = false) => {
    if (await copy(text)) {
      Toast.success(t(message || '复制成功！'));
      // Track token key copy event
      if (isTokenKey) {
        TokenAnalytics.trackTokenKeyCopied();
      }
    } else {
      Toast.error(t('复制失败，请手动复制'));
    }
  };

  const envConfig = `# 环境变量配置
ANTHROPIC_API_KEY=${tokenKey}
ANTHROPIC_BASE_URL=${baseURL}`;

  return (
    <Modal
      visible={visible}
      onCancel={onClose}
      footer={
        <div className='flex justify-end'>
          <Button type='primary' onClick={onClose}>
            {t('完成')}
          </Button>
        </div>
      }
      closeOnEsc
      width={680}
      bodyStyle={{ padding: '24px' }}
    >
      {/* Success Header */}
      <div className='text-center mb-6'>
        <div className='inline-flex items-center justify-center w-16 h-16 rounded-full bg-green-50 mb-4'>
          <IconTickCircle
            size='extra-large'
            className='text-green-500'
            style={{ fontSize: '32px' }}
          />
        </div>
        <Title heading={4} className='mb-1'>
          {t('令牌创建成功！')}
        </Title>
        <Text type='tertiary' size='small'>
          {t('请妥善保管您的令牌密钥，关闭后将无法再次查看')}
        </Text>
      </div>

      {/* Token Name */}
      <div className='mb-4'>
        <Text strong className='block mb-2 text-sm'>
          {t('令牌名称')}
        </Text>
        <div className='bg-gray-50 dark:bg-gray-800 px-4 py-3 rounded-lg border border-gray-200 dark:border-gray-700'>
          <Text>{tokenData.name}</Text>
        </div>
      </div>

      {/* Token Key - Full display with word break */}
      <div className='mb-4'>
        <div className='flex items-center justify-between mb-2'>
          <Text strong className='text-sm'>
            {t('令牌密钥')}
          </Text>
          <Button
            icon={<IconCopy />}
            size='small'
            theme='borderless'
            onClick={() => handleCopy(tokenKey, '令牌密钥已复制', true)}
          >
            {t('复制')}
          </Button>
        </div>
        <div className='bg-gray-50 dark:bg-gray-800 px-4 py-3 rounded-lg border border-gray-200 dark:border-gray-700'>
          <code
            className='text-sm font-mono break-all select-all'
            style={{ wordBreak: 'break-all', lineHeight: '1.6' }}
          >
            {tokenKey}
          </code>
        </div>
      </div>

      {/* Environment Variables - Scrollable code block */}
      <div>
        <div className='flex items-center justify-between mb-2'>
          <Text strong className='text-sm'>
            {t('环境变量配置')}
          </Text>
          <Button
            icon={<IconCopy />}
            size='small'
            theme='borderless'
            onClick={() => handleCopy(envConfig, '环境变量配置已复制')}
          >
            {t('复制')}
          </Button>
        </div>
        <div
          className='bg-gray-900 dark:bg-gray-950 rounded-lg border border-gray-700 overflow-hidden'
          style={{ maxHeight: '180px' }}
        >
          <pre
            className='p-4 m-0 text-sm overflow-auto'
            style={{
              maxHeight: '180px',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-all',
            }}
          >
            <code className='text-green-400 font-mono'>{envConfig}</code>
          </pre>
        </div>
      </div>
    </Modal>
  );
};

export default TokenCreatedSuccess;
