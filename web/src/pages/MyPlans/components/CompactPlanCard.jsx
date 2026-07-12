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
  Button,
  Card,
  Popconfirm,
  Progress,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { ArrowRight, Clock3, Lock, Unlock } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../../helpers';
import {
  canLockPlan,
  canSetCurrent,
  getInactiveKind,
  isQueuedPlan,
  isUserLocked,
  planDisplayName,
  planTypeKey,
  quotaSummary,
} from '../utils';

const { Text, Title } = Typography;
const isPending = (action, planId, type) =>
  action?.planId === planId && action?.type === type;

const inactiveLabels = {
  expired: '已过期',
  disabled: '已停用',
  completed: '已用完',
  forfeited: '已作废',
  revoked: '已回收',
};

const expirationText = (plan, t, inactive) => {
  if (!inactive && Number(plan.started_at) === 0) {
    return t('切换后开始计时');
  }
  if (!Number(plan.expires_at)) return t('永久有效');
  const days = Math.ceil((Number(plan.expires_at) - Date.now()) / 86400000);
  if (days <= 0) return t('已过期');
  return t('剩余 {{days}} 天', { days });
};

const CompactPlanCard = ({
  plan,
  section,
  pendingAction,
  onSwitch,
  onLock,
  onUnlock,
  onOpenDetails,
}) => {
  const { t, i18n } = useTranslation();
  const quota = quotaSummary(plan);
  const switchEligible = canSetCurrent(plan);
  const switchAllowed = switchEligible && plan.can_switch === 1;
  const lockEligible = canLockPlan(plan);
  const userLocked = isUserLocked(plan);
  const adminLocked = plan.locked === 1 && !userLocked;
  const adminLockMessage =
    plan.locked_reason || t('该套餐由管理员锁定，无法自行解锁');
  const queued = isQueuedPlan(plan) || Number(plan.queue_position) > 0;
  const muted = section === 'inactive';
  const inactiveKind = getInactiveKind(plan);
  const switchPending = isPending(pendingAction, plan.id, 'switch');
  const lockPending = isPending(pendingAction, plan.id, 'lock');
  const unlockPending = isPending(pendingAction, plan.id, 'unlock');
  const actionBusy = Boolean(pendingAction);
  const open = () => onOpenDetails(plan.id);
  const onKeyDown = (event) => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      open();
    }
  };

  const switchButton = (
    <Button
      className='!min-h-10'
      type='primary'
      theme={switchAllowed ? 'solid' : 'light'}
      disabled={!switchAllowed || (actionBusy && !switchPending)}
      loading={switchPending}
      icon={<ArrowRight size={15} />}
    >
      {t('设为当前')}
    </Button>
  );

  return (
    <Card
      className={[
        '!rounded-lg border border-semi-color-border shadow-sm',
        'min-h-[210px] transition-colors duration-200 hover:border-semi-color-primary',
        'motion-reduce:transition-none motion-reduce:duration-0',
        muted ? 'bg-semi-color-fill-0' : '',
      ].join(' ')}
      bodyStyle={{ padding: 16 }}
    >
      <article className='flex min-h-[178px] flex-col'>
        <div
          className={[
            'flex flex-1 cursor-pointer flex-col rounded p-1 text-left',
            'focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2',
            'focus-visible:outline-semi-color-focus-border',
          ].join(' ')}
          role='button'
          tabIndex={0}
          onClick={open}
          onKeyDown={onKeyDown}
          aria-label={t('查看 {{name}} 详情', {
            name: planDisplayName(plan),
          })}
        >
          <div className='flex items-start justify-between gap-2'>
            <div className='min-w-0 flex-1'>
              <Title
                heading={6}
                className='m-0 truncate'
                title={planDisplayName(plan)}
              >
                {planDisplayName(plan) || t('未知套餐')}
              </Title>
              <div className='mt-2 flex flex-wrap gap-1.5'>
                <Tag size='small'>{t(planTypeKey(plan))}</Tag>
                {inactiveKind && (
                  <Tag size='small'>{t(inactiveLabels[inactiveKind])}</Tag>
                )}
                {queued && (
                  <Tag size='small' color='blue'>
                    {t('队列 #{{position}}', {
                      position: plan.queue_position,
                    })}
                  </Tag>
                )}
                {userLocked && (
                  <Tag size='small' color='orange'>
                    {t('你已锁定')}
                  </Tag>
                )}
                {adminLocked && (
                  <Tag size='small' color='red'>
                    {t('管理员锁定')}
                  </Tag>
                )}
              </div>
            </div>
            <Text type='tertiary' size='small' className='shrink-0'>
              {t('优先级')} {plan.plan_priority ?? plan.plan?.priority ?? 0}
            </Text>
          </div>

          <div className='mt-4'>
            <div className='mb-1 flex justify-between gap-2 text-xs'>
              <span>{renderQuota(quota.remaining)}</span>
              <span>{renderQuota(quota.total)}</span>
            </div>
            <Progress percent={quota.remainingPercent} showInfo={false} />
            <Text
              size='small'
              type={
                Number(plan.expires_at) > 0 &&
                Number(plan.expires_at) - Date.now() <= 7 * 86400000
                  ? 'warning'
                  : 'tertiary'
              }
              className='mt-2 block'
            >
              <Clock3 size={14} className='mr-1 inline' />
              {expirationText(plan, t, muted)}
            </Text>
          </div>

          {section === 'queued' && (
            <Text type='tertiary' size='small' className='mt-2'>
              {t('前面套餐用完后自动激活')}
              {Number(plan.estimated_activation_time) > 0
                ? ` · ${t('预计激活')}: ${new Date(
                    plan.estimated_activation_time,
                  ).toLocaleDateString(i18n.language)}`
                : ''}
            </Text>
          )}
          {section === 'locked' && queued && (
            <Text type='warning' size='small' className='mt-2'>
              {t('锁定期间不会被自动激活')}
            </Text>
          )}
        </div>

        {section !== 'inactive' && (
          <div className='mt-auto flex min-h-10 flex-wrap items-end gap-2 pt-4'>
            {switchEligible &&
              (switchAllowed ? (
                <Popconfirm
                  title={t('确认切换到此套餐？')}
                  content={t('切换后将使用此套餐的额度和渠道配置')}
                  onConfirm={() => onSwitch(plan.id)}
                >
                  {switchButton}
                </Popconfirm>
              ) : (
                <Tooltip content={t('管理员已禁止切换')}>
                  <span tabIndex={0}>{switchButton}</span>
                </Tooltip>
              ))}
            {lockEligible && (
              <Popconfirm
                title={t('确认锁定此套餐？')}
                content={t('锁定期间将不会消费此套餐的额度，也不会被自动切换')}
                onConfirm={() => onLock(plan.id)}
              >
                <Button
                  className='!min-h-10'
                  icon={<Lock size={15} />}
                  disabled={actionBusy && !lockPending}
                  loading={lockPending}
                >
                  {t('锁定')}
                </Button>
              </Popconfirm>
            )}
            {userLocked && (
              <Button
                className='!min-h-10'
                icon={<Unlock size={15} />}
                disabled={actionBusy && !unlockPending}
                loading={unlockPending}
                onClick={() => onUnlock(plan.id)}
              >
                {t('解锁')}
              </Button>
            )}
            {adminLocked && (
              <Tooltip content={adminLockMessage}>
                <span tabIndex={0} className='min-w-0 max-w-full'>
                  <Text
                    type='tertiary'
                    size='small'
                    className='block max-w-full'
                    ellipsis={{ showTooltip: false }}
                  >
                    {adminLockMessage}
                  </Text>
                </span>
              </Tooltip>
            )}
          </div>
        )}
      </article>
    </Card>
  );
};

export default CompactPlanCard;
