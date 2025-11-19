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
} from '@douyinfe/semi-ui';
import { IconCheckCircleStroked } from '@douyinfe/semi-icons';
import { useNavigate } from 'react-router-dom';

const { Title, Text, Paragraph } = Typography;

/**
 * Get started step of onboarding wizard
 * Shows completion message and login button
 */
const GetStartedStep = ({ createdToken, onComplete }) => {
  const navigate = useNavigate();

  // Check if user is logged in
  const isLoggedIn = !!localStorage.getItem('user');

  /**
   * Handle login button click
   */
  const handleLogin = () => {
    onComplete();

    // Only navigate to login if user is not logged in
    if (!isLoggedIn) {
      navigate('/login');
    }
    // If already logged in, just close the wizard (via onComplete)
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
          您已成功完成设置,现在可以开始使用 API 了
        </Paragraph>
      </div>

      {/* Token info banner */}
      {createdToken && (
        <Banner
          type='success'
          description={
            <div>
              <Text strong>令牌名称: </Text>
              <Text>{createdToken.name}</Text>
              <br />
              <Text strong>令牌密钥: </Text>
              <Text code copyable>
                sk-{createdToken.key}
              </Text>
            </div>
          }
          style={{ marginBottom: 24 }}
        />
      )}

      {/* Action button */}
      <Button
        theme='solid'
        type='primary'
        size='large'
        onClick={handleLogin}
        block
      >
        {isLoggedIn ? '完成' : '登录系统'}
      </Button>
    </div>
  );
};

export default GetStartedStep;
