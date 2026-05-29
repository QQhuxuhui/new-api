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
  Empty,
} from '@douyinfe/semi-ui';
import { API, showError, showSuccess } from '../../../../helpers';
import { TokenAnalytics } from '../../../../helpers/analytics';
import { getGroupIcon } from './groupIcons';

const { Title, Text } = Typography;

// Group categories shown as columns. Each group is bucketed by matching its
// name prefix (case-insensitive) against `prefixes`; anything that matches no
// prefix falls into the trailing '其他' (other) category. Add a prefix here to
// introduce a new column.
const CATEGORIES = [
  { key: 'claude', label: 'Claude', prefixes: ['claude'] },
  { key: 'codex', label: 'Codex', prefixes: ['codex'] },
  { key: 'gemini', label: 'Gemini', prefixes: ['gemini'] },
  { key: 'other', label: '其他', prefixes: [] },
];

/**
 * Bucket groups into the fixed categories by name prefix.
 * @param {{name: string}[]} groups
 * @returns {Record<string, object[]>} category key -> groups
 */
const categorizeGroups = (groups) => {
  const buckets = Object.fromEntries(CATEGORIES.map((c) => [c.key, []]));
  for (const group of groups) {
    const name = (group.name || '').toLowerCase();
    const matched = CATEGORIES.find(
      (c) => c.prefixes.length && c.prefixes.some((p) => name.startsWith(p)),
    );
    buckets[matched ? matched.key : 'other'].push(group);
  }
  return buckets;
};

const QuickCreateTokenModal = ({
  visible,
  onSuccess,
  onCancel,
  onSwitchMode,
  initialGroup,
  t,
}) => {
  const [currentStep, setCurrentStep] = useState(1);
  // selectedGroup is the group name, used directly as the token's group
  const [selectedGroup, setSelectedGroup] = useState(null);
  const [tokenName, setTokenName] = useState('');
  const [loading, setLoading] = useState(false);
  const [nameError, setNameError] = useState('');
  const [startTime, setStartTime] = useState(null);
  // availableGroups: [{ name, desc, ratio, is_parent, children }]
  const [availableGroups, setAvailableGroups] = useState([]);
  const [groupsLoading, setGroupsLoading] = useState(true);

  // Fetch the user's available groups and turn each into a selectable card
  const fetchGroups = async () => {
    setGroupsLoading(true);
    try {
      // User-accessible endpoint (admin-only /api/group/ is not usable here)
      const res = await API.get('/api/user/self/groups');
      if (res && res.data && res.data.success && res.data.data) {
        // GetUserGroups returns { groupName: { ratio, desc, is_parent?, children? } }
        const groups = Object.entries(res.data.data).map(([name, info]) => ({
          name,
          desc: info?.desc || '',
          ratio: info?.ratio,
          is_parent: info?.is_parent || false,
          children: info?.children || [],
        }));
        setAvailableGroups(groups);
      } else {
        console.warn('Failed to fetch user groups, using fallback');
        setAvailableGroups([]);
      }
    } catch (error) {
      console.warn('Failed to fetch groups:', error);
      setAvailableGroups([]);
    } finally {
      setGroupsLoading(false);
    }
  };

  // Re-fetch groups every time the modal opens. The modal stays mounted in
  // some parents (e.g. TokensActions), so a one-shot mount fetch would mean a
  // failed/empty first response is never retried on subsequent opens.
  useEffect(() => {
    if (visible) {
      fetchGroups();
    }
  }, [visible]);

  // Reset state when modal opens/closes
  useEffect(() => {
    if (visible) {
      // If an initial group is provided, skip step 1 and go directly to step 2
      if (initialGroup) {
        setCurrentStep(2);
        setSelectedGroup(initialGroup);
        TokenAnalytics.trackTypeSelected(initialGroup);
      } else {
        setCurrentStep(1);
        setSelectedGroup(null);
      }
      setTokenName('');
      setNameError('');
      setStartTime(Date.now()); // Track start time for analytics
    }
  }, [visible, initialGroup]);

  const handleGroupSelect = (groupName) => {
    TokenAnalytics.trackTypeSelected(groupName);
    setSelectedGroup(groupName);
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

    const payload = {
      name: tokenName.trim(),
      group: selectedGroup || '', // Use the selected group directly
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
        TokenAnalytics.trackQuickCreateSuccess(selectedGroup, timeSpentSeconds);

        showSuccess(t('令牌创建成功！'));
        onSuccess(data);
      } else {
        // Track failure
        TokenAnalytics.trackQuickCreateFailed(selectedGroup, message);
        showError(t(message));
      }
    } catch (error) {
      // Track failure
      TokenAnalytics.trackQuickCreateFailed(
        selectedGroup,
        error.message || 'Network error',
      );
      showError(error.message || t('创建失败'));
    } finally {
      setLoading(false);
    }
  };

  const renderStep1 = () => {
    const buckets = categorizeGroups(availableGroups);

    return (
      <div>
        <div className='text-center mb-6'>
          <Progress percent={(1 / 2) * 100} showInfo={false} />
          <Text type='tertiary' className='mt-2 block'>
            {t('步骤')} 1/2
          </Text>
        </div>

        <Title heading={4} className='mb-4 text-center'>
          {t('选择分组')}
        </Title>

        {groupsLoading ? (
          <div className='flex justify-center py-8'>
            <Spin size='large' />
          </div>
        ) : availableGroups.length === 0 ? (
          <Empty
            className='py-6'
            description={t('暂无可用分组，请使用高级配置创建令牌')}
          />
        ) : (
          <div className='grid grid-cols-2 md:grid-cols-4 gap-3'>
            {CATEGORIES.map((category) => {
              const groups = buckets[category.key];
              return (
                <div key={category.key} className='flex flex-col'>
                  <div
                    className='flex items-center justify-center gap-2 mb-3 pb-2'
                    style={{
                      borderBottom: '1px solid var(--semi-color-border)',
                    }}
                  >
                    <span className='text-blue-500 flex items-center'>
                      {getGroupIcon(category.key, 18)}
                    </span>
                    <Text strong>{t(category.label)}</Text>
                  </div>

                  {groups.length === 0 ? (
                    <Text
                      type='tertiary'
                      size='small'
                      className='block text-center py-4'
                    >
                      {t('暂无可用分组')}
                    </Text>
                  ) : (
                    <div className='space-y-2'>
                      {groups.map((group) => (
                        <Card
                          key={group.name}
                          className='cursor-pointer transition-all hover:shadow-md hover:border-blue-500'
                          onClick={() => handleGroupSelect(group.name)}
                          bodyStyle={{ padding: '12px' }}
                        >
                          <div className='text-center'>
                            <Text
                              strong
                              className='block'
                              style={{ fontSize: 15 }}
                            >
                              {group.name}
                            </Text>
                            {group.desc && (
                              <Text
                                type='tertiary'
                                size='small'
                                className='block mt-1'
                              >
                                {group.desc}
                              </Text>
                            )}
                          </div>
                        </Card>
                      ))}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
    );
  };

  const renderStep2 = () => {
    const groupInfo = availableGroups.find((g) => g.name === selectedGroup) || {
      name: selectedGroup,
      desc: '',
    };
    const displayName = groupInfo.desc || groupInfo.name;

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
          <div className='mb-4 flex items-center'>
            <span className='mr-2 text-blue-500'>
              {getGroupIcon(selectedGroup)}
            </span>
            <Text strong>{displayName}</Text>
          </div>

          <div className='mb-4'>
            <Text strong className='block mb-2'>
              {t('预设配置')}:
            </Text>
            <div className='space-y-1'>
              <div className='flex items-center'>
                <Text type='tertiary'>• {t('分组')}:</Text>
                <Tag color='blue' size='small' className='ml-2'>
                  {selectedGroup}
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
            id='quick-create-token-name'
            name='tokenName'
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
            autoComplete='off'
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
      width={currentStep === 1 ? 920 : 600}
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
