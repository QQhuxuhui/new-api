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

import {
  Banner,
  Button,
  Card,
  Progress,
  Switch,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { AlertTriangle, CheckCircle2, Clock3, Pin, PinOff } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../../helpers';
import { planDisplayName, planTypeKey, quotaSummary } from '../utils';

const { Text, Title } = Typography;
const pending = (action, planId, type) =>
  action?.planId === planId && action?.type === type;

const CurrentPlanHero = ({
  plan,
  quotaStatus,
  pendingAction,
  onToggleAutoSwitch,
  onClearPinned,
}) => {
  const { t, i18n } = useTranslation();
  if (!plan) return null;

  const quota = quotaSummary(plan);
  const dailyLimit = Number(quotaStatus?.daily_quota_limit || 0);
  const dailyUsed = Number(quotaStatus?.daily_quota_used || 0);
  const dailyRemaining = Number(
    quotaStatus?.daily_quota_remaining ?? Math.max(dailyLimit - dailyUsed, 0),
  );
  const dailyPercent =
    dailyLimit > 0
      ? Math.min(100, Math.max(0, (dailyUsed / dailyLimit) * 100))
      : 0;
  const resetText = quotaStatus?.daily_reset_time
    ? new Date(quotaStatus.daily_reset_time * 1000).toLocaleTimeString(
        i18n.language,
      )
    : t('明日 00:00');
  const waitSeconds = Number(quotaStatus?.rate_limit_wait_seconds || 0);
  const rateLimitMessage =
    quotaStatus?.rate_limit_message || t('速率限制：请稍后重试');
  const canToggle = plan.can_toggle_auto === 1 && plan.locked !== 1;
  const autoPending = pending(pendingAction, plan.id, 'auto-switch');
  const clearPending = pending(pendingAction, plan.id, 'clear-pin');
  const actionBusy = Boolean(pendingAction);
  const clearPinHelp =
    plan.locked === 1
      ? plan.locked_reason || t('该套餐由管理员锁定，无法自行解锁')
      : t(
          '系统不会自动升级更换；额度用尽或故障仍自动处理。点击「解除」恢复自动调度，会一并开启自动切换。',
        );

  return (
    <Card className='!rounded-lg border border-semi-color-border shadow-sm'>
      <div className='flex flex-col gap-4'>
        <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
          <div className='min-w-0'>
            <div className='flex flex-wrap items-center gap-2'>
              <Title heading={4} className='m-0 min-w-0 break-words'>
                {planDisplayName(plan) || t('未知套餐')}
              </Title>
              <Tag color='blue' size='small'>
                <CheckCircle2 size={14} /> {t('当前使用')}
              </Tag>
              {plan.pinned === 1 && (
                <Tag color='orange' size='small'>
                  <Pin size={14} /> {t('手动指定')}
                </Tag>
              )}
            </div>
            <div className='mt-2 flex flex-wrap gap-2'>
              <Tag>
                {t('优先级')}: {plan.plan_priority ?? plan.plan?.priority ?? 0}
              </Tag>
              <Tag>{t(planTypeKey(plan))}</Tag>
            </div>
          </div>
          {plan.pinned === 1 && (
            <Tooltip content={clearPinHelp}>
              <span tabIndex={plan.locked === 1 ? 0 : undefined}>
                <Button
                  size='small'
                  theme='light'
                  className='!min-h-10'
                  icon={<PinOff size={15} />}
                  disabled={plan.locked === 1 || (actionBusy && !clearPending)}
                  loading={clearPending}
                  onClick={() => onClearPinned(plan.id)}
                >
                  {t('解除')}
                </Button>
              </span>
            </Tooltip>
          )}
        </div>

        <div>
          <div className='mb-2 flex items-center justify-between gap-3'>
            <Text strong>{t('总额度')}</Text>
            <Text>
              {renderQuota(quota.remaining)} / {renderQuota(quota.total)}
            </Text>
          </div>
          <Progress percent={quota.remainingPercent} showInfo={false} />
          <div className='mt-1 flex justify-between gap-3 text-xs text-semi-color-text-2'>
            <span>
              {t('已使用')}: {renderQuota(quota.used)}
            </span>
            <span>
              {t('剩余')}: {renderQuota(quota.remaining)}
            </span>
          </div>
        </div>

        {dailyLimit > 0 && (
          <div className='rounded-lg bg-semi-color-fill-0 p-3'>
            <div className='mb-2 flex flex-wrap items-center justify-between gap-2'>
              <Text strong>{t('今日额度')}</Text>
              <Text size='small'>
                <Clock3 size={14} className='mr-1 inline' /> {t('重置时间')}:{' '}
                {resetText}
              </Text>
            </div>
            <Progress percent={dailyPercent} showInfo={false} />
            <div className='mt-1 flex justify-between gap-3 text-xs text-semi-color-text-2'>
              <span>
                {renderQuota(dailyUsed)} / {renderQuota(dailyLimit)}
              </span>
              <span>
                {t('剩余')}: {renderQuota(dailyRemaining)}
              </span>
            </div>
          </div>
        )}

        {quotaStatus?.rate_limited && (
          <Banner
            type='warning'
            icon={<AlertTriangle size={16} />}
            description={`${rateLimitMessage} (${Math.ceil(waitSeconds / 60)} ${t('分钟')})`}
          />
        )}

        <div className='flex flex-col gap-2 border-t border-semi-color-border pt-3 sm:flex-row sm:items-center sm:justify-between'>
          <div>
            <Text strong>{t('自动切换')}</Text>
            <Text type='tertiary' size='small' className='block'>
              {canToggle
                ? t('控制额度耗尽救援与渠道故障转移')
                : t('自动切换由管理员控制')}
            </Text>
          </div>
          <Tooltip
            content={
              canToggle
                ? t('控制额度耗尽救援与渠道故障转移')
                : t('自动切换由管理员控制')
            }
          >
            <label
              className='inline-flex min-h-10 min-w-10 items-center justify-center'
              tabIndex={!canToggle ? 0 : undefined}
            >
              <Switch
                checked={plan.auto_switch === 1}
                disabled={!canToggle || (actionBusy && !autoPending)}
                loading={autoPending}
                aria-label={t('自动切换')}
                onChange={(checked) => onToggleAutoSwitch(plan.id, checked)}
              />
            </label>
          </Tooltip>
        </div>
      </div>
    </Card>
  );
};

export default CurrentPlanHero;
