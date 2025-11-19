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
  Card,
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
      width={600}
      bodyStyle={{ padding: '24px' }}
    >
      <div className='text-center mb-6'>
        <IconTickCircle
          size='extra-large'
          className='text-green-500 mb-3'
          style={{ fontSize: '48px' }}
        />
        <Title heading={3} className='mb-2'>
          {t('令牌创建成功！')}
        </Title>
      </div>

      {/* Token Information */}
      <Card className='mb-4'>
        <div className='mb-3'>
          <Text strong className='block mb-2'>
            {t('令牌名称')}:
          </Text>
          <div className='flex items-center justify-between bg-gray-50 p-3 rounded'>
            <Text>{tokenData.name}</Text>
          </div>
        </div>

        <div>
          <Text strong className='block mb-2'>
            {t('令牌密钥')}:
          </Text>
          <div className='flex items-center justify-between bg-gray-50 p-3 rounded'>
            <Text
              code
              copyable={{
                onCopy: () => handleCopy(tokenKey, '令牌密钥已复制', true),
              }}
              className='flex-1 overflow-hidden'
              ellipsis={{ showTooltip: true }}
            >
              {tokenKey}
            </Text>
          </div>
        </div>
      </Card>

      {/* Environment Variables */}
      <Card>
        <div className='flex items-center justify-between mb-2'>
          <Text strong>{t('环境变量配置')}</Text>
          <Button
            icon={<IconCopy />}
            size='small'
            onClick={() => handleCopy(envConfig, '环境变量配置已复制')}
          >
            {t('复制')}
          </Button>
        </div>
        <pre className='bg-gray-50 p-3 rounded text-sm overflow-x-auto'>
          <code>{envConfig}</code>
        </pre>
      </Card>
    </Modal>
  );
};

export default TokenCreatedSuccess;
