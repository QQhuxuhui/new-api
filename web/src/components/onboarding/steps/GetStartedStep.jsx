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
import {
  Button,
  Typography,
  Space,
  Tabs,
  TabPane,
  Card,
  Toast,
  Banner,
} from '@douyinfe/semi-ui';
import { IconCheckCircleStroked, IconCopy } from '@douyinfe/semi-icons';
import { copy } from '../../../helpers';

const { Title, Text, Paragraph } = Typography;

/**
 * Get started step of onboarding wizard
 * Shows code examples and completion message
 */
const GetStartedStep = ({ createdToken, onComplete }) => {
  const [activeTab, setActiveTab] = useState('python');

  // Get the base URL for API
  const baseURL = window.location.origin;
  const apiKey = createdToken?.key ? `sk-${createdToken.key}` : 'YOUR_API_KEY';

  /**
   * Generate code example based on language
   */
  const getCodeExample = (language) => {
    switch (language) {
      case 'python':
        return `from openai import OpenAI

# 初始化客户端
client = OpenAI(
    api_key="${apiKey}",
    base_url="${baseURL}/v1"
)

# 发送请求
response = client.chat.completions.create(
    model="gpt-3.5-turbo",
    messages=[
        {"role": "user", "content": "Hello, how are you?"}
    ]
)

print(response.choices[0].message.content)`;

      case 'nodejs':
        return `import OpenAI from 'openai';

// 初始化客户端
const client = new OpenAI({
  apiKey: '${apiKey}',
  baseURL: '${baseURL}/v1'
});

// 发送请求
async function main() {
  const response = await client.chat.completions.create({
    model: 'gpt-3.5-turbo',
    messages: [
      { role: 'user', content: 'Hello, how are you?' }
    ]
  });

  console.log(response.choices[0].message.content);
}

main();`;

      case 'curl':
        return `curl ${baseURL}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer ${apiKey}" \\
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [
      {
        "role": "user",
        "content": "Hello, how are you?"
      }
    ]
  }'`;

      default:
        return '';
    }
  };

  /**
   * Handle copy code to clipboard
   */
  const handleCopyCode = () => {
    const code = getCodeExample(activeTab);
    copy(code);
    Toast.success('代码已复制到剪贴板');
  };

  /**
   * Handle finish button click
   */
  const handleFinish = () => {
    onComplete();
  };

  return (
    <div style={{ padding: '20px 0' }}>
      {/* Success message */}
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <IconCheckCircleStroked
          size="extra-large"
          style={{ fontSize: 64, color: 'var(--semi-color-success)', marginBottom: 16 }}
        />
        <Title heading={4}>恭喜! 设置完成</Title>
        <Paragraph type="tertiary" style={{ marginTop: 8 }}>
          您已成功完成设置,现在可以开始使用 API 了
        </Paragraph>
      </div>

      {/* Token info banner */}
      {createdToken && (
        <Banner
          type="success"
          description={
            <div>
              <Text strong>令牌名称: </Text>
              <Text>{createdToken.name}</Text>
              <br />
              <Text strong>令牌密钥: </Text>
              <Text code copyable>sk-{createdToken.key}</Text>
            </div>
          }
          style={{ marginBottom: 24 }}
        />
      )}

      {/* Code examples */}
      <Card
        title="快速开始示例代码"
        style={{ marginBottom: 24 }}
        headerExtraContent={
          <Button
            icon={<IconCopy />}
            theme="borderless"
            type="tertiary"
            onClick={handleCopyCode}
          >
            复制代码
          </Button>
        }
      >
        <Tabs
          type="line"
          activeKey={activeTab}
          onChange={setActiveTab}
        >
          <TabPane tab="Python" itemKey="python">
            <pre
              style={{
                backgroundColor: 'var(--semi-color-fill-0)',
                padding: 16,
                borderRadius: 6,
                overflow: 'auto',
                fontSize: 13,
                fontFamily: 'monospace',
              }}
            >
              <code>{getCodeExample('python')}</code>
            </pre>
          </TabPane>

          <TabPane tab="Node.js" itemKey="nodejs">
            <pre
              style={{
                backgroundColor: 'var(--semi-color-fill-0)',
                padding: 16,
                borderRadius: 6,
                overflow: 'auto',
                fontSize: 13,
                fontFamily: 'monospace',
              }}
            >
              <code>{getCodeExample('nodejs')}</code>
            </pre>
          </TabPane>

          <TabPane tab="cURL" itemKey="curl">
            <pre
              style={{
                backgroundColor: 'var(--semi-color-fill-0)',
                padding: 16,
                borderRadius: 6,
                overflow: 'auto',
                fontSize: 13,
                fontFamily: 'monospace',
              }}
            >
              <code>{getCodeExample('curl')}</code>
            </pre>
          </TabPane>
        </Tabs>
      </Card>

      {/* Action buttons */}
      <Space vertical spacing="medium" style={{ width: '100%' }}>
        <Button
          theme="solid"
          type="primary"
          size="large"
          onClick={handleFinish}
          block
        >
          完成设置
        </Button>
        <Button
          theme="borderless"
          type="tertiary"
          onClick={() => window.open('/docs', '_blank')}
          block
        >
          查看完整文档
        </Button>
      </Space>
    </div>
  );
};

export default GetStartedStep;
