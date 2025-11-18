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
import { Modal, Button, Typography } from '@douyinfe/semi-ui';
import { IconBolt, IconSetting } from '@douyinfe/semi-icons';
import { TokenAnalytics } from '../../../../helpers/analytics';

const { Title, Text } = Typography;

const TokenCreateModeSelector = ({ visible, onSelect, onCancel, t }) => {
  const handleModeSelect = (mode) => {
    TokenAnalytics.trackModeSelected(mode);
    onSelect(mode);
  };

  return (
    <Modal
      visible={visible}
      onCancel={onCancel}
      footer={null}
      closeOnEsc
      width={600}
      bodyStyle={{ padding: '24px' }}
    >
      <div className='text-center mb-6'>
        <Title heading={3} className='mb-2'>
          {t('创建令牌')}
        </Title>
        <Text type='tertiary'>{t('选择创建方式')}</Text>
      </div>

      <div className='grid grid-cols-1 md:grid-cols-2 gap-4'>
        {/* Quick Create Mode */}
        <div
          className='border rounded-lg p-6 cursor-pointer transition-all hover:shadow-lg hover:border-blue-500'
          onClick={() => handleModeSelect('quick')}
          role='button'
          tabIndex={0}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              handleModeSelect('quick');
            }
          }}
        >
          <div className='flex flex-col items-center text-center'>
            <div className='mb-4 p-3 bg-blue-50 rounded-full'>
              <IconBolt size='extra-large' className='text-blue-500' />
            </div>
            <Title heading={4} className='mb-2'>
              {t('快速创建')}
            </Title>
            <Text type='tertiary' className='mb-4'>
              {t('使用预设配置，30秒内完成创建')}
            </Text>
            <ul className='text-left text-sm text-gray-600 mb-4 space-y-1'>
              <li>• {t('预设常用配置')}</li>
              <li>• {t('无限额度')}</li>
              <li>• {t('永不过期')}</li>
              <li>• {t('无访问限制')}</li>
            </ul>
            <Button theme='solid' type='primary' className='w-full'>
              {t('快速创建')}
            </Button>
          </div>
        </div>

        {/* Advanced Configuration Mode */}
        <div
          className='border rounded-lg p-6 cursor-pointer transition-all hover:shadow-lg hover:border-purple-500'
          onClick={() => handleModeSelect('advanced')}
          role='button'
          tabIndex={0}
          onKeyDown={(e) => {
            if (e.key === 'Enter' || e.key === ' ') {
              handleModeSelect('advanced');
            }
          }}
        >
          <div className='flex flex-col items-center text-center'>
            <div className='mb-4 p-3 bg-purple-50 rounded-full'>
              <IconSetting size='extra-large' className='text-purple-500' />
            </div>
            <Title heading={4} className='mb-2'>
              {t('高级配置')}
            </Title>
            <Text type='tertiary' className='mb-4'>
              {t('自定义所有参数，精确控制')}
            </Text>
            <ul className='text-left text-sm text-gray-600 mb-4 space-y-1'>
              <li>• {t('自定义额度')}</li>
              <li>• {t('设置过期时间')}</li>
              <li>• {t('模型限制')}</li>
              <li>• {t('IP白名单')}</li>
            </ul>
            <Button theme='borderless' type='primary' className='w-full'>
              {t('高级配置')}
            </Button>
          </div>
        </div>
      </div>
    </Modal>
  );
};

export default TokenCreateModeSelector;
