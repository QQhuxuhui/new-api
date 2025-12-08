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

import React, { useState } from 'react';
import { Button, Typography, Space, Card } from '@douyinfe/semi-ui';
import { IconCode, IconTerminal, IconSetting } from '@douyinfe/semi-icons';
import { Gemini } from '@lobehub/icons';
import { useTranslation } from 'react-i18next';
import QuickCreateTokenModal from '../../table/tokens/modals/QuickCreateTokenModal';
import { OnboardingAnalytics } from '../../../helpers/analytics';

const { Title, Text, Paragraph } = Typography;

/**
 * Create token step of onboarding wizard
 * Guides user through creating their first API token
 */
const CreateTokenStep = ({ onNext, onPrev, onSkip }) => {
  const { t } = useTranslation();
  const [showQuickCreate, setShowQuickCreate] = useState(false);
  const [selectedTokenType, setSelectedTokenType] = useState(null);

  /**
   * Handle token type card click
   */
  const handleTokenTypeClick = (type) => {
    setSelectedTokenType(type);
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
    setSelectedTokenType(null);
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

  const tokenTypes = [
    {
      id: 'claude-code',
      name: 'Claude Code',
      icon: <IconCode size='extra-large' />,
      description: '用于 Claude Code 开发工具',
      color: 'var(--semi-color-primary)',
    },
    {
      id: 'codex',
      name: 'Codex',
      icon: <IconTerminal size='extra-large' />,
      description: '用于代码生成和补全',
      color: 'var(--semi-color-success)',
    },
    {
      id: 'gemini',
      name: 'Gemini',
      icon: <Gemini.Color size='extra-large' />,
      description: '用于 Google Gemini',
      color: 'var(--semi-color-warning)',
    },
  ];

  return (
    <div style={{ padding: '20px 0' }}>
      {/* Title */}
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <Title heading={4}>创建 API 令牌</Title>
        <Paragraph type='tertiary' style={{ marginTop: 8 }}>
          选择一个令牌类型快速创建
        </Paragraph>
      </div>

      {/* Token type cards */}
      <Space
        vertical
        spacing='medium'
        style={{ width: '100%', marginBottom: 24 }}
      >
        {tokenTypes.map((type) => (
          <Card
            key={type.id}
            shadows='hover'
            style={{
              border: '1px solid var(--semi-color-border)',
              cursor: 'pointer',
              transition: 'all 0.3s',
            }}
            bodyStyle={{ padding: 20 }}
            onClick={() => handleTokenTypeClick(type.id)}
          >
            <div style={{ display: 'flex', alignItems: 'center', gap: 16 }}>
              <div style={{ color: type.color }}>{type.icon}</div>
              <div style={{ flex: 1 }}>
                <Text
                  strong
                  style={{ fontSize: 16, display: 'block', marginBottom: 4 }}
                >
                  {type.name}
                </Text>
                <Text type='tertiary' size='small'>
                  {type.description}
                </Text>
              </div>
              <Button theme='solid' type='primary'>
                创建
              </Button>
            </div>
          </Card>
        ))}
      </Space>

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
          使用高级配置
        </Button>
      </div>

      {/* Navigation buttons */}
      <Space style={{ width: '100%', justifyContent: 'space-between' }}>
        <Button theme='borderless' type='tertiary' onClick={onPrev}>
          上一步
        </Button>
        <Button theme='borderless' type='tertiary' onClick={handleSkip}>
          跳过此步
        </Button>
      </Space>

      {/* Quick create modal */}
      {showQuickCreate && (
        <QuickCreateTokenModal
          visible={showQuickCreate}
          initialTokenType={selectedTokenType}
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
