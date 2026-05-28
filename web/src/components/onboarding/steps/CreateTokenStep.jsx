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
  Button,
  Typography,
  Space,
  Card,
  Tag,
  Spin,
  Empty,
} from '@douyinfe/semi-ui';
import { IconSetting } from '@douyinfe/semi-icons';
import { useTranslation } from 'react-i18next';
import QuickCreateTokenModal from '../../table/tokens/modals/QuickCreateTokenModal';
import { getGroupIcon } from '../../table/tokens/modals/groupIcons';
import { API } from '../../../helpers';
import { OnboardingAnalytics } from '../../../helpers/analytics';

const { Title, Text, Paragraph } = Typography;

/**
 * Create token step of onboarding wizard
 * Guides user through creating their first API token
 */
const CreateTokenStep = ({ onNext, onPrev, onSkip }) => {
  const { t } = useTranslation();
  const [showQuickCreate, setShowQuickCreate] = useState(false);
  const [selectedGroup, setSelectedGroup] = useState(null);
  // availableGroups: [{ name, desc, ratio, is_parent, children }]
  const [availableGroups, setAvailableGroups] = useState([]);
  const [groupsLoading, setGroupsLoading] = useState(true);

  // Fetch the user's available groups; each becomes a selectable card
  useEffect(() => {
    const fetchGroups = async () => {
      setGroupsLoading(true);
      try {
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
          setAvailableGroups([]);
        }
      } catch (error) {
        console.warn('Failed to fetch groups:', error);
        setAvailableGroups([]);
      } finally {
        setGroupsLoading(false);
      }
    };
    fetchGroups();
  }, []);

  /**
   * Handle group card click
   */
  const handleGroupClick = (groupName) => {
    setSelectedGroup(groupName);
    setShowQuickCreate(true);
  };

  /**
   * Handle successful token creation from quick create modal
   */
  const handleTokenCreated = (tokenData) => {
    // Track token creation in onboarding
    OnboardingAnalytics.trackTokenCreated();

    setShowQuickCreate(false);
    onNext({ createdToken: tokenData });
  };

  /**
   * Handle quick create modal close without creating token
   */
  const handleQuickCreateClose = () => {
    setShowQuickCreate(false);
    setSelectedGroup(null);
  };

  /**
   * Handle switch to advanced mode
   */
  const handleSwitchMode = () => {
    setShowQuickCreate(false);
    // Navigate to tokens page for advanced configuration
    window.location.href = '/console/token';
  };

  /**
   * Handle skip step
   */
  const handleSkip = () => {
    onSkip({ skipped: true });
  };

  return (
    <div style={{ padding: '20px 0' }}>
      {/* Title */}
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <Title heading={4}>{t('创建 API 令牌')}</Title>
        <Paragraph type='tertiary' style={{ marginTop: 8 }}>
          {t('选择一个分组快速创建')}
        </Paragraph>
      </div>

      {/* Group cards */}
      {groupsLoading ? (
        <div
          style={{
            display: 'flex',
            justifyContent: 'center',
            padding: '32px 0',
          }}
        >
          <Spin size='large' />
        </div>
      ) : availableGroups.length === 0 ? (
        <Empty
          style={{ padding: '24px 0' }}
          description={t('暂无可用分组，请使用高级配置创建令牌')}
        />
      ) : (
        <Space
          vertical
          spacing='medium'
          style={{ width: '100%', marginBottom: 24 }}
        >
          {availableGroups.map((group) => (
            <Card
              key={group.name}
              shadows='hover'
              style={{
                width: '100%',
                border: '1px solid var(--semi-color-border)',
                cursor: 'pointer',
                transition: 'all 0.3s',
              }}
              bodyStyle={{ padding: 20 }}
              onClick={() => handleGroupClick(group.name)}
            >
              <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
                <div style={{ color: 'var(--semi-color-primary)' }}>
                  {getGroupIcon(group.name)}
                </div>
                <div style={{ flex: 1 }}>
                  <Text
                    strong
                    style={{ fontSize: 16, display: 'block', marginBottom: 4 }}
                  >
                    {group.desc || group.name}
                  </Text>
                  <Tag color='blue' size='small'>
                    {group.name}
                  </Tag>
                </div>
                <Button theme='solid' type='primary'>
                  {t('创建')}
                </Button>
              </div>
            </Card>
          ))}
        </Space>
      )}

      {/* Advanced configuration link */}
      <div style={{ textAlign: 'center', marginBottom: 32 }}>
        <Button
          theme='borderless'
          type='tertiary'
          icon={<IconSetting />}
          onClick={() => {
            // Navigate to tokens page for advanced configuration
            window.location.href = '/console/token';
          }}
        >
          {t('使用高级配置')}
        </Button>
      </div>

      {/* Navigation buttons */}
      <Space style={{ width: '100%', justifyContent: 'space-between' }}>
        <Button theme='borderless' type='tertiary' onClick={onPrev}>
          {t('上一步')}
        </Button>
        <Button theme='borderless' type='tertiary' onClick={handleSkip}>
          {t('跳过此步')}
        </Button>
      </Space>

      {/* Quick create modal */}
      {showQuickCreate && (
        <QuickCreateTokenModal
          visible={showQuickCreate}
          initialGroup={selectedGroup}
          onSuccess={handleTokenCreated}
          onCancel={handleQuickCreateClose}
          onSwitchMode={handleSwitchMode}
          t={t}
        />
      )}
    </div>
  );
};

export default CreateTokenStep;
