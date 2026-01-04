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
  Banner,
  Tag,
} from '@douyinfe/semi-ui';
import {
  IconTicketCodeStroked,
  IconCreditCard,
  IconBox,
} from '@douyinfe/semi-icons';
import { useNavigate } from 'react-router-dom';
import { API, showError, showSuccess, renderQuota } from '../../../helpers';
import { UserContext } from '../../../context/User';
import { OnboardingAnalytics } from '../../../helpers/analytics';

const { Title, Text, Paragraph } = Typography;

/**
 * Usage mode selection step of onboarding wizard
 * Allows users to choose between subscription plans, pay-as-you-go, or redemption code
 */
const UsageModeStep = ({ onNext, onPrev, onSkip }) => {
  const navigate = useNavigate();
  const [userState, userDispatch] = useContext(UserContext);

  const [redemptionCode, setRedemptionCode] = useState('');
  const [isRedeeming, setIsRedeeming] = useState(false);

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
        // Handle backward compatible response: data.quota for new format
        const redeemedQuota = typeof data === 'object' ? data.quota : data;
        const isPlanRedemption = data?.mode === 'plan';

        if (isPlanRedemption) {
          showSuccess(`套餐兑换成功! 套餐: ${data.plan_name}, 额度: ${renderQuota(redeemedQuota)}`);
        } else {
          showSuccess('兑换成功! 获得额度: ' + renderQuota(redeemedQuota));
        }

        // Track redemption code usage in onboarding
        OnboardingAnalytics.trackRedemptionCodeUsed();

        // Update user quota in context (only for user_balance mode)
        if (!isPlanRedemption && userState.user) {
          const updatedUser = {
            ...userState.user,
            quota: userState.user.quota + redeemedQuota,
          };
          userDispatch({ type: 'login', payload: updatedUser });
        }

        setRedemptionCode('');

        // Advance to next step after successful redemption
        setTimeout(() => {
          onNext({ topupAmount: redeemedQuota, method: isPlanRedemption ? 'plan_redemption' : 'redemption_code' });
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
   * Handle view plans - navigate to plans page
   */
  const handleViewPlans = () => {
    // Record selection so向导可以前进到下一步
    onNext({ method: 'plan_subscription', destination: '/plans' });
    navigate('/plans');
  };

  /**
   * Handle go to topup - navigate to wallet page
   */
  const handleGoToTopup = () => {
    // 记录按量付费选择并继续向导
    onNext({ method: 'payg_topup', destination: '/console/topup' });
    navigate('/console/topup');
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
        <Title heading={4}>选择使用模式</Title>
        <Paragraph type='tertiary' style={{ marginTop: 8 }}>
          选择一种方式为您的账户获取额度
        </Paragraph>
      </div>

      {/* Info banner */}
      <Banner
        type='info'
        description='新用户赠送的额度可以直接使用，无需充值'
        style={{ marginBottom: 24 }}
      />

      {/* Subscription Plans Option */}
      <Card
        shadows='hover'
        style={{
          marginBottom: 16,
          border: '1px solid var(--semi-color-border)',
          cursor: 'pointer',
        }}
        onClick={handleViewPlans}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <IconBox
              size='large'
              style={{ color: 'var(--semi-color-primary)' }}
            />
            <div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <Text strong style={{ fontSize: 16 }}>
                  套餐订阅
                </Text>
                <Tag color='blue' size='small'>推荐</Tag>
              </div>
              <Text type='tertiary' size='small'>
                日卡/周卡/月卡等，固定额度更划算
              </Text>
            </div>
          </div>
          <Button theme='solid' type='primary' onClick={(e) => { e.stopPropagation(); handleViewPlans(); }}>
            查看套餐
          </Button>
        </div>
      </Card>

      {/* Pay-as-you-go Option */}
      <Card
        shadows='hover'
        style={{
          marginBottom: 16,
          border: '1px solid var(--semi-color-border)',
          cursor: 'pointer',
        }}
        onClick={handleGoToTopup}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <IconCreditCard
              size='large'
              style={{ color: 'var(--semi-color-success)' }}
            />
            <div>
              <Text strong style={{ fontSize: 16 }}>
                按量付费
              </Text>
              <br />
              <Text type='tertiary' size='small'>
                充值后按实际使用量扣费，不限时，灵活使用
              </Text>
            </div>
          </div>
          <Button theme='light' type='primary' onClick={(e) => { e.stopPropagation(); handleGoToTopup(); }}>
            前往充值
          </Button>
        </div>
      </Card>

      {/* Redemption Code Option */}
      <Card
        shadows='hover'
        style={{
          marginBottom: 32,
          border: '1px solid var(--semi-color-border)',
        }}
      >
        <Space vertical spacing='medium' style={{ width: '100%' }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
            <IconTicketCodeStroked
              size='large'
              style={{ color: 'var(--semi-color-warning)' }}
            />
            <div>
              <Text strong style={{ fontSize: 16 }}>
                兑换码充值
              </Text>
              <br />
              <Text type='tertiary' size='small'>
                输入兑换码即可快速获取额度
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
              type='warning'
              onClick={handleRedeem}
              loading={isRedeeming}
              disabled={!redemptionCode.trim()}
            >
              兑换
            </Button>
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

export default UsageModeStep;
