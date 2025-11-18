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

import React, { useState, useEffect } from 'react';
import {
  Modal,
  Button,
  Typography,
  Input,
  Space,
  Progress,
  Card,
  Tag,
  Spin,
} from '@douyinfe/semi-ui';
import { IconCode, IconTerminal } from '@douyinfe/semi-icons';
import { API, showError, showSuccess } from '../../../../helpers';
import { TokenAnalytics } from '../../../../helpers/analytics';

const { Title, Text } = Typography;

const TOKEN_TYPES = {
  'claude-code': {
    id: 'claude-code',
    name: 'Claude Code',
    icon: <IconCode size='extra-large' />,
    description: '用于 Claude Code 开发',
    features: ['无限额度', '永不过期', '无访问限制'],
    group: 'default',
  },
  codex: {
    id: 'codex',
    name: 'Codex',
    icon: <IconTerminal size='extra-large' />,
    description: '用于 Codex 开发',
    features: ['无限额度', '永不过期', '无访问限制'],
    group: 'default',
  },
};

const QuickCreateTokenModal = ({
  visible,
  onSuccess,
  onCancel,
  onSwitchMode,
  t,
}) => {
  const [currentStep, setCurrentStep] = useState(1);
  const [selectedType, setSelectedType] = useState(null);
  const [tokenName, setTokenName] = useState('');
  const [loading, setLoading] = useState(false);
  const [nameError, setNameError] = useState('');
  const [startTime, setStartTime] = useState(null);

  // Reset state when modal opens/closes
  useEffect(() => {
    if (visible) {
      setCurrentStep(1);
      setSelectedType(null);
      setTokenName('');
      setNameError('');
      setStartTime(Date.now()); // Track start time for analytics
    }
  }, [visible]);

  const handleTypeSelect = (typeId) => {
    TokenAnalytics.trackTypeSelected(typeId);
    setSelectedType(typeId);
    setCurrentStep(2);
  };

  const handleBack = () => {
    setCurrentStep(1);
    setTokenName('');
    setNameError('');
  };

  const validateName = (name) => {
    if (!name || name.trim() === '') {
      setNameError(t('请输入令牌名称'));
      return false;
    }
    if (name.length > 30) {
      setNameError(t('名称最多30个字符'));
      return false;
    }
    setNameError('');
    return true;
  };

  const handleCreate = async () => {
    if (!validateName(tokenName)) {
      return;
    }

    setLoading(true);
    const tokenType = TOKEN_TYPES[selectedType];

    const payload = {
      name: tokenName.trim(),
      group: tokenType.group,
      unlimited_quota: true,
      remain_quota: 0,
      expired_time: -1,
      model_limits_enabled: false,
      model_limits: '',
      allow_ips: '',
    };

    try {
      const res = await API.post('/api/token/', payload);
      const { success, message, data } = res.data;

      if (success) {
        // Track success with time spent (convert milliseconds to seconds)
        const timeSpentMs = startTime ? Date.now() - startTime : 0;
        const timeSpentSeconds = Math.round(timeSpentMs / 1000);
        TokenAnalytics.trackQuickCreateSuccess(selectedType, timeSpentSeconds);

        showSuccess(t('令牌创建成功！'));
        onSuccess(data);
      } else {
        // Track failure
        TokenAnalytics.trackQuickCreateFailed(selectedType, message);
        showError(t(message));
      }
    } catch (error) {
      // Track failure
      TokenAnalytics.trackQuickCreateFailed(
        selectedType,
        error.message || 'Network error',
      );
      showError(error.message || t('创建失败'));
    } finally {
      setLoading(false);
    }
  };

  const renderStep1 = () => (
    <div>
      <div className='text-center mb-6'>
        <Progress percent={(1 / 2) * 100} showInfo={false} />
        <Text type='tertiary' className='mt-2 block'>
          {t('步骤')} 1/2
        </Text>
      </div>

      <Title heading={4} className='mb-4 text-center'>
        {t('选择令牌类型')}
      </Title>

      <div className='grid grid-cols-1 md:grid-cols-2 gap-4'>
        {Object.values(TOKEN_TYPES).map((type) => (
          <Card
            key={type.id}
            className='cursor-pointer transition-all hover:shadow-lg hover:border-blue-500'
            onClick={() => handleTypeSelect(type.id)}
            bodyStyle={{ padding: '20px' }}
          >
            <div className='flex flex-col items-center text-center'>
              <div className='mb-3 p-3 bg-blue-50 rounded-full text-blue-500'>
                {type.icon}
              </div>
              <Title heading={5} className='mb-2'>
                {t(type.name)}
              </Title>
              <Text type='tertiary' className='mb-3'>
                {t(type.description)}
              </Text>
              <div className='space-y-1'>
                {type.features.map((feature, idx) => (
                  <Tag key={idx} color='blue' size='small'>
                    {t(feature)}
                  </Tag>
                ))}
              </div>
            </div>
          </Card>
        ))}
      </div>
    </div>
  );

  const renderStep2 = () => {
    const tokenType = TOKEN_TYPES[selectedType];

    return (
      <div>
        <div className='text-center mb-6'>
          <Progress percent={(2 / 2) * 100} showInfo={false} />
          <Text type='tertiary' className='mt-2 block'>
            {t('步骤')} 2/2
          </Text>
        </div>

        <Title heading={4} className='mb-4 text-center'>
          {t('配置令牌')}
        </Title>

        <Card className='mb-4'>
          <div className='mb-4'>
            <Text strong>{t('令牌类型')}:</Text>
            <Text className='ml-2'>{t(tokenType.name)}</Text>
          </div>

          <div className='mb-4'>
            <Text strong className='block mb-2'>
              {t('预设配置')}:
            </Text>
            <div className='space-y-1'>
              <div className='flex items-center'>
                <Text type='tertiary'>• {t('分组')}:</Text>
                <Tag color='blue' size='small' className='ml-2'>
                  {tokenType.group}
                </Tag>
              </div>
              <div className='flex items-center'>
                <Text type='tertiary'>• {t('额度')}:</Text>
                <Tag color='green' size='small' className='ml-2'>
                  {t('无限额度')}
                </Tag>
              </div>
              <div className='flex items-center'>
                <Text type='tertiary'>• {t('过期时间')}:</Text>
                <Tag color='orange' size='small' className='ml-2'>
                  {t('永不过期')}
                </Tag>
              </div>
              <div className='flex items-center'>
                <Text type='tertiary'>• {t('访问限制')}:</Text>
                <Tag size='small' className='ml-2'>
                  {t('无限制')}
                </Tag>
              </div>
            </div>
          </div>
        </Card>

        <div className='mb-4'>
          <Text strong className='block mb-2'>
            {t('令牌名称')} <Text type='danger'>*</Text>
          </Text>
          <Input
            placeholder={t('请输入令牌名称')}
            value={tokenName}
            onChange={(value) => {
              setTokenName(value);
              if (nameError) {
                validateName(value);
              }
            }}
            onBlur={() => validateName(tokenName)}
            maxLength={30}
            showClear
            validateStatus={nameError ? 'error' : 'default'}
          />
          {nameError && (
            <Text type='danger' size='small' className='mt-1 block'>
              {nameError}
            </Text>
          )}
        </div>

        <Space className='w-full justify-between'>
          <Button onClick={handleBack}>{t('上一步')}</Button>
          <Button
            theme='solid'
            type='primary'
            onClick={handleCreate}
            loading={loading}
            disabled={!tokenName.trim()}
          >
            {t('创建令牌')}
          </Button>
        </Space>
      </div>
    );
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
      <Spin spinning={loading}>
        {currentStep === 1 ? renderStep1() : renderStep2()}

        <div className='mt-4 text-center'>
          <Button
            type='tertiary'
            size='small'
            onClick={() => {
              TokenAnalytics.trackSwitchedToAdvanced(currentStep);
              onSwitchMode();
            }}
          >
            {t('切换到高级配置')}
          </Button>
        </div>
      </Spin>
    </Modal>
  );
};

export default QuickCreateTokenModal;
