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

import React, {
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import {
  Button,
  Card,
  Empty,
  Popconfirm,
  Progress,
  Spin,
  Tag,
  Tooltip,
  Typography,
} from '@douyinfe/semi-ui';
import { ArrowRight, Box, Clock3, Lock, RefreshCw, Unlock } from 'lucide-react';
import { API, renderQuota, showError, showSuccess } from '../../helpers';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import CurrentPlanHero from './components/CurrentPlanHero';
import DailyPoolCard from './components/DailyPoolCard';
import WalletCard from './components/WalletCard';
import {
  canLockPlan,
  canSetCurrent,
  enrichPlansWithQueueMetadata,
  getInactiveKind,
  groupPlans,
  isPlansRouteEnabled,
  isQueuedPlan,
  isUserLocked,
  planDisplayName,
  quotaSummary,
} from './utils';

const { Text, Title } = Typography;
const actionPending = (action, planId, type) =>
  action?.planId === planId && action?.type === type;

const inactiveLabels = {
  expired: '已过期',
  disabled: '已停用',
  completed: '已用完',
  forfeited: '已作废',
  revoked: '已回收',
};

const TemporaryPlanCard = ({
  plan,
  pendingAction,
  onSwitch,
  onLock,
  onUnlock,
}) => {
  const { t } = useTranslation();
  const quota = quotaSummary(plan);
  const queued = isQueuedPlan(plan) || Number(plan.queue_position) > 0;
  const inactiveKind = getInactiveKind(plan);
  const userLocked = isUserLocked(plan);
  const adminLocked = plan.locked === 1 && !userLocked;
  const switchEligible = canSetCurrent(plan);
  const switchAllowed = switchEligible && plan.can_switch === 1;
  const lockEligible = canLockPlan(plan);
  const switchPending = actionPending(pendingAction, plan.id, 'switch');
  const lockPending = actionPending(pendingAction, plan.id, 'lock');
  const unlockPending = actionPending(pendingAction, plan.id, 'unlock');
  const actionBusy = Boolean(pendingAction);
  const switchButton = (
    <Button
      className='!min-h-10'
      type='primary'
      theme={switchAllowed ? 'solid' : 'light'}
      icon={<ArrowRight size={15} />}
      disabled={!switchAllowed || (actionBusy && !switchPending)}
      loading={switchPending}
    >
      {t('设为当前')}
    </Button>
  );

  return (
    <Card className='!rounded-lg border border-semi-color-border shadow-sm'>
      <article className='flex flex-col gap-4'>
        <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
          <div className='min-w-0'>
            <Title heading={6} className='m-0 break-words'>
              {planDisplayName(plan) || t('未知套餐')}
            </Title>
            <div className='mt-2 flex flex-wrap gap-2'>
              {inactiveKind && <Tag>{t(inactiveLabels[inactiveKind])}</Tag>}
              {queued && (
                <Tag color='blue'>
                  <Clock3 size={14} />{' '}
                  {t('队列 #{{position}}', {
                    position: plan.queue_position,
                  })}
                </Tag>
              )}
              {userLocked && <Tag color='orange'>{t('你已锁定')}</Tag>}
              {adminLocked && <Tag color='red'>{t('管理员锁定')}</Tag>}
              <Tag>
                {t('优先级')} {plan.plan_priority ?? plan.plan?.priority ?? 0}
              </Tag>
            </div>
          </div>
          <Text strong>{renderQuota(quota.remaining)}</Text>
        </div>

        <div>
          <div className='mb-1 flex justify-between gap-3 text-xs text-semi-color-text-2'>
            <span>{t('剩余')}</span>
            <span>{renderQuota(quota.total)}</span>
          </div>
          <Progress percent={quota.remainingPercent} showInfo={false} />
        </div>

        {!inactiveKind && (
          <div className='flex flex-wrap gap-2'>
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
              <Tooltip
                content={
                  plan.locked_reason || t('该套餐由管理员锁定，无法自行解锁')
                }
              >
                <span tabIndex={0} className='min-w-0'>
                  <Text type='tertiary' size='small' ellipsis>
                    {plan.locked_reason ||
                      t('该套餐由管理员锁定，无法自行解锁')}
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

const MyPlans = () => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [userState] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);
  const [initialLoading, setInitialLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [pendingAction, setPendingAction] = useState(null);
  const [userPlans, setUserPlans] = useState([]);
  const [quotaStatus, setQuotaStatus] = useState(null);
  const [billingStatus, setBillingStatus] = useState(null);
  const refreshEpoch = useRef(0);

  const rechargeDisabled = Boolean(statusState?.status?.recharge_disabled);
  const walletBalance = userState?.user?.quota || 0;

  const responseMessage = useCallback(
    (error) =>
      error?.response?.data?.message || error?.message || t('操作失败，请重试'),
    [t],
  );

  const notifyRequestError = useCallback(
    (error) => {
      if (error?.response?.status === 401) {
        showError(error);
        return;
      }
      showError(responseMessage(error));
    },
    [responseMessage],
  );

  const refreshData = useCallback(
    async ({ initial = false, explicit = false, fresh = false } = {}) => {
      const requestId = ++refreshEpoch.current;
      if (initial) setInitialLoading(true);
      if (explicit) setRefreshing(true);
      const requestConfig = {
        skipErrorHandler: true,
        disableDuplicate: fresh,
      };
      const [plansResult, quotaResult, billingResult] =
        await Promise.allSettled([
          API.get('/api/my_plans/', requestConfig),
          API.get('/api/my_plans/quota-status', requestConfig),
          API.get('/api/my_plans/billing-status', requestConfig),
        ]);

      if (requestId !== refreshEpoch.current) return;

      if (
        plansResult.status === 'fulfilled' &&
        plansResult.value.data.success
      ) {
        setUserPlans(plansResult.value.data.data?.plans || []);
      } else if (plansResult.status === 'rejected') {
        notifyRequestError(plansResult.reason);
      } else {
        showError(plansResult.value.data.message || t('操作失败，请重试'));
      }

      if (
        quotaResult.status === 'fulfilled' &&
        quotaResult.value.data.success
      ) {
        setQuotaStatus(quotaResult.value.data.data || null);
      } else {
        setQuotaStatus(null);
        console.error(
          'Failed to load quota status:',
          quotaResult.status === 'rejected'
            ? quotaResult.reason
            : quotaResult.value.data.message,
        );
      }

      if (
        billingResult.status === 'fulfilled' &&
        billingResult.value.data.success
      ) {
        setBillingStatus(billingResult.value.data.data || null);
      } else {
        setBillingStatus(null);
        console.error(
          'Failed to load billing status:',
          billingResult.status === 'rejected'
            ? billingResult.reason
            : billingResult.value.data.message,
        );
      }

      setInitialLoading(false);
      setRefreshing(false);
    },
    [notifyRequestError, t],
  );

  useEffect(() => {
    refreshData({ initial: true });
  }, [refreshData]);

  const runPlanAction = async ({ type, planId, request, successMessage }) => {
    setPendingAction({ type, planId });
    try {
      const response = await request();
      if (response.data.success) {
        showSuccess(successMessage);
      } else {
        showError(response.data.message);
      }
    } catch (error) {
      notifyRequestError(error);
    } finally {
      await refreshData({ fresh: true });
      setPendingAction(null);
    }
  };

  const handleSwitchPlan = (planId) =>
    runPlanAction({
      type: 'switch',
      planId,
      request: () =>
        API.post(
          '/api/my_plans/switch',
          { user_plan_id: planId },
          { skipErrorHandler: true },
        ),
      successMessage: t(
        '已切换到该套餐。系统不会自动更换你的选择；额度用尽或渠道故障时仍会自动处理。',
      ),
    });

  const handleToggleAutoSwitch = (planId, enabled) =>
    runPlanAction({
      type: 'auto-switch',
      planId,
      request: () =>
        API.put(
          `/api/my_plans/${planId}/auto_switch`,
          { enabled },
          { skipErrorHandler: true },
        ),
      successMessage: t(enabled ? '已开启自动切换' : '已关闭自动切换'),
    });

  const handleClearPinned = (planId) =>
    runPlanAction({
      type: 'clear-pin',
      planId,
      request: () =>
        API.put(
          `/api/my_plans/${planId}/auto_switch`,
          { enabled: true },
          { skipErrorHandler: true },
        ),
      successMessage: t('已恢复系统自动调度'),
    });

  const handleLockPlan = (planId) =>
    runPlanAction({
      type: 'lock',
      planId,
      request: () =>
        API.post(
          `/api/my_plans/${planId}/lock`,
          {},
          { skipErrorHandler: true },
        ),
      successMessage: t('套餐已锁定'),
    });

  const handleUnlockPlan = (planId) =>
    runPlanAction({
      type: 'unlock',
      planId,
      request: () =>
        API.post(
          `/api/my_plans/${planId}/unlock`,
          {},
          { skipErrorHandler: true },
        ),
      successMessage: t('套餐已解锁'),
    });

  const enrichedPlans = useMemo(
    () =>
      enrichPlansWithQueueMetadata(
        userPlans,
        billingStatus?.queued_plans || [],
      ),
    [userPlans, billingStatus?.queued_plans],
  );
  const groupedPlans = useMemo(
    () => groupPlans(enrichedPlans),
    [enrichedPlans],
  );
  const plansRouteEnabled = useMemo(
    () =>
      isPlansRouteEnabled(
        statusState?.status?.HeaderNavModules,
        statusState?.status !== undefined,
      ),
    [statusState?.status],
  );
  const nonCurrentPlans = useMemo(
    () => [
      ...groupedPlans.available,
      ...groupedPlans.queued,
      ...groupedPlans.locked,
      ...groupedPlans.inactive,
    ],
    [groupedPlans],
  );
  const interactionPending =
    refreshing && !pendingAction
      ? { type: 'refresh', planId: -1 }
      : pendingAction;

  return (
    <div className='min-h-screen bg-semi-color-bg-0 pb-12'>
      <main className='mx-auto max-w-7xl px-4 pb-6 pt-[72px] sm:px-6 lg:px-8'>
        <header className='mb-5 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between'>
          <div>
            <Title heading={3} className='m-0'>
              {t('我的套餐')}
            </Title>
            <Text type='tertiary'>{t('管理您的订阅计划与额度使用详情')}</Text>
          </div>
          <Button
            className='!min-h-10'
            icon={<RefreshCw size={16} />}
            loading={refreshing}
            disabled={Boolean(pendingAction)}
            onClick={() => refreshData({ explicit: true, fresh: true })}
          >
            {t('刷新数据')}
          </Button>
        </header>

        <Spin spinning={initialLoading} tip={t('加载套餐信息...')}>
          <div className='min-h-[240px] space-y-5'>
            <CurrentPlanHero
              plan={groupedPlans.current}
              quotaStatus={quotaStatus}
              pendingAction={interactionPending}
              onToggleAutoSwitch={handleToggleAutoSwitch}
              onClearPinned={handleClearPinned}
            />
            <DailyPoolCard pool={billingStatus?.daily_pool} />

            {userPlans.length === 0 ? (
              <Empty
                image={<Box size={48} className='text-semi-color-text-2' />}
                title={t('暂无套餐')}
                description={t(
                  '您当前没有任何可用的套餐订阅，可以通过按量付费使用服务。',
                )}
              >
                {plansRouteEnabled && !rechargeDisabled && (
                  <Button
                    className='!min-h-10'
                    type='primary'
                    theme='solid'
                    onClick={() => navigate('/plans')}
                  >
                    {t('去购买')}
                  </Button>
                )}
              </Empty>
            ) : (
              <div className='space-y-4'>
                {nonCurrentPlans.map((plan) => (
                  <TemporaryPlanCard
                    key={plan.id}
                    plan={plan}
                    pendingAction={interactionPending}
                    onSwitch={handleSwitchPlan}
                    onLock={handleLockPlan}
                    onUnlock={handleUnlockPlan}
                  />
                ))}
              </div>
            )}

            <WalletCard
              balance={walletBalance}
              rechargeDisabled={rechargeDisabled || !plansRouteEnabled}
              onRecharge={() => navigate('/plans?category=payg')}
            />
          </div>
        </Spin>

        <footer className='mt-8 text-center text-sm text-semi-color-text-2'>
          {t('套餐额度仅供参考，具体扣费以实际使用量为准')}
        </footer>
      </main>
    </div>
  );
};

export default MyPlans;
