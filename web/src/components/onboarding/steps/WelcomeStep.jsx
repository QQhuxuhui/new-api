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
import { Button, Typography, Space, Checkbox, Card } from '@douyinfe/semi-ui';
import { IconPlay } from '@douyinfe/semi-icons';

const { Title, Text, Paragraph } = Typography;

/**
 * Welcome step of onboarding wizard
 * Introduces the wizard and sets expectations
 */
const WelcomeStep = ({ onNext, onSkip }) => {
  const [dontShowAgain, setDontShowAgain] = useState(false);

  const handleGetStarted = () => {
    onNext({ dontShowAgain });
  };

  const handleSkip = () => {
    onSkip({ dontShowAgain });
  };

  return (
    <div style={{ textAlign: 'center', padding: '20px 0' }}>
      {/* Icon */}
      <div style={{ marginBottom: 24 }}>
        <IconPlay size="extra-large" style={{ fontSize: 64, color: 'var(--semi-color-primary)' }} />
      </div>

      {/* Title */}
      <Title heading={3} style={{ marginBottom: 16 }}>
        欢迎使用 API 服务平台
      </Title>

      {/* Description */}
      <Paragraph style={{ fontSize: 16, marginBottom: 32, color: 'var(--semi-color-text-1)' }}>
        让我们花 2 分钟时间,帮助您快速开始使用 API 服务
      </Paragraph>

      {/* Features list */}
      <Card
        shadows="hover"
        style={{
          marginBottom: 32,
          textAlign: 'left',
          backgroundColor: 'var(--semi-color-fill-0)',
          border: '1px solid var(--semi-color-border)',
        }}
      >
        <Space vertical align="start" spacing="medium">
          <div>
            <Text strong style={{ fontSize: 15 }}>
              📋 完成以下 3 个简单步骤:
            </Text>
          </div>
          <div style={{ paddingLeft: 16 }}>
            <Space vertical spacing="small" align="start">
              <Text type="secondary">1. 充值账户 (可选)</Text>
              <Text type="secondary">2. 创建 API 令牌</Text>
              <Text type="secondary">3. 获取使用示例代码</Text>
            </Space>
          </div>
          <div style={{ marginTop: 8 }}>
            <Text type="tertiary" size="small">
              ⏱️ 预计耗时: 约 2 分钟
            </Text>
          </div>
        </Space>
      </Card>

      {/* Action buttons */}
      <Space vertical spacing="medium" style={{ width: '100%' }}>
        <Button
          theme="solid"
          type="primary"
          size="large"
          onClick={handleGetStarted}
          block
        >
          开始使用
        </Button>
        <Button
          theme="borderless"
          type="tertiary"
          onClick={handleSkip}
          block
        >
          稍后再说
        </Button>
      </Space>

      {/* Don't show again checkbox */}
      <div style={{ marginTop: 24 }}>
        <Checkbox
          checked={dontShowAgain}
          onChange={(e) => setDontShowAgain(e.target.checked)}
        >
          <Text type="tertiary" size="small">
            不再显示此向导
          </Text>
        </Checkbox>
      </div>
    </div>
  );
};

export default WelcomeStep;
