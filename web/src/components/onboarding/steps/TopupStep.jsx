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

import React, { useState, useContext } from 'react';
import {
  Button,
  Typography,
  Space,
  Input,
  Card,
  Spin,
  Banner,
} from '@douyinfe/semi-ui';
import {
  IconTicketCodeStroked,
  IconUserCardPhone,
} from '@douyinfe/semi-icons';
import { API, showError, showSuccess, renderQuota } from '../../../helpers';
import { UserContext } from '../../../context/User';
import { StatusContext } from '../../../context/Status';
import { OnboardingAnalytics } from '../../../helpers/analytics';

const { Title, Text, Paragraph } = Typography;

/**
 * Top-up step of onboarding wizard
 * Allows users to add credits to their account
 */
const TopupStep = ({ onNext, onPrev, onSkip }) => {
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);

  const [redemptionCode, setRedemptionCode] = useState('');
  const [isRedeeming, setIsRedeeming] = useState(false);

  const xianyuShopLink = statusState?.status?.xianyu_shop_link || '';

  /**
   * Handle redemption code submission
   */
  const handleRedeem = async () => {
    if (!redemptionCode.trim()) {
      showError('请输入兑换码');
      return;
    }

    setIsRedeeming(true);
    try {
      const res = await API.post('/api/user/topup', {
        key: redemptionCode,
      });
      const { success, message, data } = res.data;
      if (success) {
        showSuccess('兑换成功! 获得额度: ' + renderQuota(data));

        // Track redemption code usage in onboarding
        OnboardingAnalytics.trackRedemptionCodeUsed();

        // Update user quota in context
        if (userState.user) {
          const updatedUser = {
            ...userState.user,
            quota: userState.user.quota + data,
          };
          userDispatch({ type: 'login', payload: updatedUser });
        }

        setRedemptionCode('');

        // Advance to next step after successful redemption
        setTimeout(() => {
          onNext({ topupAmount: data, method: 'redemption_code' });
        }, 1000);
      } else {
        showError(message || '兑换失败');
      }
    } catch (err) {
      showError('兑换请求失败');
    } finally {
      setIsRedeeming(false);
    }
  };

  /**
   * Handle skip step
   */
  const handleSkip = () => {
    onSkip({ skipped: true });
  };

  /**
   * Open Xianyu shop in new tab
   */
  const handleXianyuShop = () => {
    if (!xianyuShopLink) {
      showError('管理员未设置闲鱼店铺链接');
      return;
    }
    window.open(xianyuShopLink, '_blank');
  };

  return (
    <div style={{ padding: '20px 0' }}>
      {/* Title */}
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <Title heading={4}>充值账户</Title>
        <Paragraph type='tertiary' style={{ marginTop: 8 }}>
          选择一种方式为您的账户充值
        </Paragraph>
      </div>

      {/* Info banner */}
      <Banner
        type='info'
        description='新用户赠送的额度可以直接使用,无需充值'
        style={{ marginBottom: 24 }}
      />

      {/* Redemption Code Option */}
      <Card
        shadows='hover'
        style={{
          marginBottom: 16,
          border: '1px solid var(--semi-color-border)',
        }}
      >
        <Space vertical spacing='medium' style={{ width: '100%' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <IconTicketCodeStroked
              size='large'
              style={{ color: 'var(--semi-color-primary)' }}
            />
            <div>
              <Text strong style={{ fontSize: 16 }}>
                兑换码充值
              </Text>
              <br />
              <Text type='tertiary' size='small'>
                输入兑换码即可快速充值
              </Text>
            </div>
          </div>

          <Space style={{ width: '100%' }}>
            <Input
              id="onboarding-redemption-code"
              name="redemptionCode"
              placeholder='请输入兑换码'
              value={redemptionCode}
              onChange={setRedemptionCode}
              onEnterPress={handleRedeem}
              disabled={isRedeeming}
              style={{ flex: 1 }}
              autoComplete="off"
            />
            <Button
              theme='solid'
              type='primary'
              onClick={handleRedeem}
              loading={isRedeeming}
              disabled={!redemptionCode.trim()}
            >
              兑换
            </Button>
          </Space>
        </Space>
      </Card>

      {/* Contact Admin or Xianyu Shop Option */}
      <Card
        shadows='hover'
        style={{
          marginBottom: 32,
          border: '1px solid var(--semi-color-border)',
        }}
      >
        <Space vertical spacing='medium' style={{ width: '100%' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <IconUserCardPhone
              size='large'
              style={{ color: 'var(--semi-color-warning)' }}
            />
            <div>
              <Text strong style={{ fontSize: 16 }}>
                联系管理员或闲鱼店铺购买
              </Text>
              <br />
              <Text type='tertiary' size='small'>
                如需帮助，请联系平台管理员或访问闲鱼店铺购买
              </Text>
            </div>
          </div>

          <Space style={{ width: '100%' }}>
            <Button theme='solid' type='secondary' block>
              联系管理员
            </Button>
            {xianyuShopLink && (
              <Button
                theme='solid'
                type='secondary'
                onClick={handleXianyuShop}
                block
              >
                闲鱼店铺
              </Button>
            )}
          </Space>
        </Space>
      </Card>

      {/* Navigation buttons */}
      <Space style={{ width: '100%', justifyContent: 'space-between' }}>
        <Button theme='borderless' type='tertiary' onClick={onPrev}>
          上一步
        </Button>
        <Button theme='borderless' type='tertiary' onClick={handleSkip}>
          跳过此步
        </Button>
      </Space>
    </div>
  );
};

export default TopupStep;
