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

import { Modal, Tag, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../../helpers';
import { isUserLocked, planDisplayName, quotaSummary } from '../utils';

const { Text, Title } = Typography;
const formatMs = (value, empty, locale) =>
  Number(value) > 0 ? new Date(Number(value)).toLocaleString(locale) : empty;

const DetailRow = ({ label, children }) => (
  <div className='min-w-0 border-b border-semi-color-border py-2 last:border-b-0'>
    <Text type='tertiary' size='small' className='block'>
      {label}
    </Text>
    <div className='mt-1 break-words'>{children}</div>
  </div>
);

const PlanDetailModal = ({ visible, plan, onClose }) => {
  const { t, i18n } = useTranslation();
  if (!plan) return null;
  const quota = quotaSummary(plan);
  const lockedBy = isUserLocked(plan) ? t('你') : t('管理员');

  return (
    <Modal
      visible={visible}
      onCancel={onClose}
      footer={null}
      width='min(680px, calc(100vw - 24px))'
      title={t('套餐详情')}
    >
      <div className='mb-3 flex flex-wrap items-center gap-2'>
        <Title heading={5} className='m-0 min-w-0 break-all'>
          {planDisplayName(plan) || t('未知套餐')}
        </Title>
        {plan.pinned === 1 && <Tag color='orange'>{t('手动指定')}</Tag>}
        {plan.locked === 1 && <Tag color='red'>{t('已锁定')}</Tag>}
      </div>
      <div className='grid grid-cols-1 gap-x-5 sm:grid-cols-2'>
        <DetailRow label={t('总额度')}>{renderQuota(quota.total)}</DetailRow>
        <DetailRow label={t('已使用')}>{renderQuota(quota.used)}</DetailRow>
        <DetailRow label={t('剩余')}>{renderQuota(quota.remaining)}</DetailRow>
        <DetailRow label={t('优先级')}>
          {plan.plan_priority ?? plan.plan?.priority ?? 0}
        </DetailRow>
        <DetailRow label={t('有效期')}>
          {Number(plan.plan_validity_days) > 0
            ? `${plan.plan_validity_days} ${t('天')}`
            : t('永久有效')}
        </DetailRow>
        <DetailRow label={t('开始时间')}>
          {formatMs(plan.started_at, t('未激活'), i18n.language)}
        </DetailRow>
        <DetailRow label={t('到期时间')}>
          {formatMs(plan.expires_at, t('永久有效'), i18n.language)}
        </DetailRow>
        <DetailRow label={t('每日限额')}>
          {Number(plan.effective_daily_limit) > 0
            ? renderQuota(plan.effective_daily_limit)
            : t('无限制')}
        </DetailRow>
        <DetailRow label={t('自动切换状态')}>
          {plan.auto_switch === 1 ? t('已开启') : t('已关闭')}
        </DetailRow>
        <DetailRow label={t('手动指定状态')}>
          {plan.pinned === 1 ? t('是') : t('否')}
        </DetailRow>
        {plan.locked === 1 && (
          <DetailRow label={t('锁定方')}>{lockedBy}</DetailRow>
        )}
        {plan.locked === 1 && (
          <DetailRow label={t('锁定原因')}>
            {plan.locked_reason || t('未设置')}
          </DetailRow>
        )}
        {plan.admin_note && (
          <DetailRow label={t('管理员备注')}>{plan.admin_note}</DetailRow>
        )}
        {Number(plan.estimated_activation_time) > 0 && (
          <DetailRow label={t('预计激活')}>
            {formatMs(
              plan.estimated_activation_time,
              t('未设置'),
              i18n.language,
            )}
          </DetailRow>
        )}
      </div>
    </Modal>
  );
};

export default PlanDetailModal;
