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

import React, { useState, useMemo } from 'react';
import { Button, Typography, Space, Checkbox, Card, Toast } from '@douyinfe/semi-ui';
import { IconCopy } from '@douyinfe/semi-icons';
import { Rocket } from 'lucide-react';

const { Title, Text, Paragraph } = Typography;

/**
 * Welcome step of onboarding wizard
 * Introduces the wizard and shows platform overview with API endpoints
 */
const WelcomeStep = ({ onNext, onSkip }) => {
  const [dontShowAgain, setDontShowAgain] = useState(false);

  // Get base URL dynamically
  const baseUrl = useMemo(() => window.location.origin, []);

  const apiEndpoints = [
    { name: 'Claude API', url: baseUrl },
    { name: 'OpenAI API', url: `${baseUrl}/v1` },
    { name: 'Gemini API', url: baseUrl },
  ];

  const handleGetStarted = () => {
    onNext({ dontShowAgain });
  };

  const handleSkip = () => {
    onSkip({ dontShowAgain });
  };

  const handleCopy = async (url) => {
    try {
      await navigator.clipboard.writeText(url);
      Toast.success('已复制到剪贴板');
    } catch (err) {
      Toast.error('复制失败');
    }
  };

  return (
    <div style={{ textAlign: 'center', padding: '20px 0' }}>
      {/* Icon */}
      <div style={{ marginBottom: 24 }}>
        <Rocket
          size={64}
          strokeWidth={1.5}
          color='var(--semi-color-primary)'
        />
      </div>

      {/* Title */}
      <Title heading={3} style={{ marginBottom: 8 }}>
        欢迎使用 Spark Code 中转API 服务平台
      </Title>

      {/* Subtitle */}
      <Paragraph
        style={{
          fontSize: 16,
          marginBottom: 24,
          color: 'var(--semi-color-text-1)',
        }}
      >
        Claude Code、Codex、Gemini 的最佳中转服务
      </Paragraph>

      {/* API Endpoints Card */}
      <Card
        shadows='hover'
        style={{
          marginBottom: 24,
          textAlign: 'left',
          backgroundColor: 'var(--semi-color-fill-0)',
          border: '1px solid var(--semi-color-border)',
        }}
      >
        <Space vertical align='start' spacing='medium' style={{ width: '100%' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Text strong style={{ fontSize: 15 }}>
              📡 API 地址
            </Text>
          </div>
          <div style={{ width: '100%' }}>
            {apiEndpoints.map((endpoint, index) => (
              <div
                key={index}
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  padding: '8px 0',
                  borderBottom: index < apiEndpoints.length - 1 ? '1px solid var(--semi-color-border)' : 'none',
                }}
              >
                <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                  <Text type='secondary' style={{ minWidth: 80 }}>{endpoint.name}:</Text>
                  <Text code style={{ fontSize: 13 }}>{endpoint.url}</Text>
                </div>
                <Button
                  theme='borderless'
                  type='tertiary'
                  size='small'
                  icon={<IconCopy />}
                  onClick={() => handleCopy(endpoint.url)}
                />
              </div>
            ))}
          </div>
        </Space>
      </Card>

      {/* Steps preview */}
      <Card
        shadows='hover'
        style={{
          marginBottom: 24,
          textAlign: 'left',
          backgroundColor: 'var(--semi-color-fill-0)',
          border: '1px solid var(--semi-color-border)',
        }}
      >
        <Space vertical align='start' spacing='medium'>
          <div>
            <Text strong style={{ fontSize: 15 }}>
              📋 完成以下 3 个简单步骤:
            </Text>
          </div>
          <div style={{ paddingLeft: 16 }}>
            <Space vertical spacing='small' align='start'>
              <Text type='secondary'>1. 选择使用模式（套餐或按量付费）</Text>
              <Text type='secondary'>2. 创建 API 令牌</Text>
              <Text type='secondary'>3. 查看安装教程</Text>
            </Space>
          </div>
          <div style={{ marginTop: 8 }}>
            <Text type='tertiary' size='small'>
              ⏱️ 预计耗时: 约 2 分钟
            </Text>
          </div>
        </Space>
      </Card>

      {/* Action buttons */}
      <Space vertical spacing='medium' style={{ width: '100%' }}>
        <Button
          theme='solid'
          type='primary'
          size='large'
          onClick={handleGetStarted}
          block
        >
          开始使用
        </Button>
        <Button theme='borderless' type='tertiary' onClick={handleSkip} block>
          稍后再说
        </Button>
      </Space>

      {/* Don't show again checkbox */}
      <div style={{ marginTop: 24 }}>
        <Checkbox
          id="onboarding-dont-show-again"
          checked={dontShowAgain}
          onChange={(e) => setDontShowAgain(e.target.checked)}
        >
          <Text type='tertiary' size='small'>
            不再显示此向导
          </Text>
        </Checkbox>
      </div>
    </div>
  );
};

export default WelcomeStep;
