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
  Button,
  Typography,
  Space,
  Banner,
  Card,
  Toast,
} from '@douyinfe/semi-ui';
import { IconCheckCircleStroked, IconCopy } from '@douyinfe/semi-icons';
import { useNavigate } from 'react-router-dom';
import { Claude, OpenAI, Gemini } from '@lobehub/icons';
import { Code2 } from 'lucide-react';

const { Title, Text, Paragraph } = Typography;

// Icon size for consistent display
const ICON_SIZE = 28;

// Tool configurations
const TOOL_CONFIGS = [
  {
    id: 'claude-code',
    name: 'Claude Code',
    icon: <Claude.Color size={ICON_SIZE} />,
    description: '终端AI编程助手',
  },
  {
    id: 'codex',
    name: 'Codex',
    icon: <OpenAI size={ICON_SIZE} />,
    description: 'AI代码补全',
  },
  {
    id: 'gemini',
    name: 'Gemini',
    icon: <Gemini.Color size={ICON_SIZE} />,
    description: 'Google AI服务',
  },
  {
    id: 'vscode',
    name: 'VSCode插件',
    icon: <Code2 size={ICON_SIZE} strokeWidth={1.5} color="var(--semi-color-primary)" />,
    description: 'IDE集成',
  },
];

/**
 * Tutorial step of onboarding wizard
 * Shows completion message and guides user to installation tutorial
 */
const TutorialStep = ({ createdToken, onComplete }) => {
  const navigate = useNavigate();

  /**
   * Handle copy token key
   */
  const handleCopyToken = async () => {
    if (createdToken?.key) {
      try {
        await navigator.clipboard.writeText(`sk-${createdToken.key}`);
        Toast.success('已复制到剪贴板');
      } catch (err) {
        Toast.error('复制失败');
      }
    }
  };

  /**
   * Handle view tutorial
   */
  const handleViewTutorial = () => {
    onComplete();
    navigate('/tutorial');
  };

  /**
   * Handle go to console
   */
  const handleGoToConsole = () => {
    onComplete();
    navigate('/console');
  };

  /**
   * Handle tool card click - navigate to tutorial
   */
  const handleToolClick = () => {
    onComplete();
    navigate('/tutorial');
  };

  return (
    <div style={{ padding: '20px 0' }}>
      {/* Success message */}
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <IconCheckCircleStroked
          size='extra-large'
          style={{
            fontSize: 64,
            color: 'var(--semi-color-success)',
            marginBottom: 16,
          }}
        />
        <Title heading={4}>恭喜! 设置完成</Title>
        <Paragraph type='tertiary' style={{ marginTop: 8 }}>
          您已完成基础设置，现在可以开始使用了
        </Paragraph>
      </div>

      {/* Token info banner */}
      {createdToken && (
        <Banner
          type='success'
          style={{ marginBottom: 24 }}
          description={
            <div>
              <div style={{ marginBottom: 8 }}>
                <Text strong>令牌创建成功</Text>
              </div>
              <div style={{ marginBottom: 4 }}>
                <Text>令牌名称: </Text>
                <Text strong>{createdToken.name}</Text>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <Text>令牌密钥: </Text>
                <Text code>sk-{createdToken.key}</Text>
                <Button
                  theme='borderless'
                  type='tertiary'
                  size='small'
                  icon={<IconCopy />}
                  onClick={handleCopyToken}
                />
              </div>
              <div style={{ marginTop: 8 }}>
                <Text type='warning' size='small'>
                  ⚠️ 请妥善保存，令牌密钥仅显示一次
                </Text>
              </div>
            </div>
          }
        />
      )}

      {/* Tool cards section */}
      <div style={{ marginBottom: 24 }}>
        <div style={{ textAlign: 'center', marginBottom: 16 }}>
          <Text strong>选择工具查看配置教程</Text>
        </div>
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(2, 1fr)',
            gap: 12,
          }}
        >
          {TOOL_CONFIGS.map((tool) => (
            <Card
              key={tool.id}
              shadows='hover'
              style={{
                border: '1px solid var(--semi-color-border)',
                cursor: 'pointer',
                transition: 'all 0.2s',
              }}
              bodyStyle={{ padding: 16 }}
              onClick={handleToolClick}
            >
              <div style={{ textAlign: 'center' }}>
                <div style={{ marginBottom: 8 }}>{tool.icon}</div>
                <Text strong style={{ display: 'block', marginBottom: 4 }}>
                  {tool.name}
                </Text>
                <Text type='tertiary' size='small'>
                  {tool.description}
                </Text>
              </div>
            </Card>
          ))}
        </div>
      </div>

      {/* Action buttons */}
      <Space vertical spacing='medium' style={{ width: '100%' }}>
        <Button
          theme='solid'
          type='primary'
          size='large'
          onClick={handleViewTutorial}
          block
        >
          查看安装教程
        </Button>
        <Button
          theme='light'
          type='tertiary'
          size='large'
          onClick={handleGoToConsole}
          block
        >
          前往控制台
        </Button>
      </Space>
    </div>
  );
};

export default TutorialStep;
