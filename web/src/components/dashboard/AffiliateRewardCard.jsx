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

import React, { useRef, useState } from 'react';
import { Button, Tag, Banner, Skeleton } from '@douyinfe/semi-ui';
import { Gift, Copy } from 'lucide-react';
import { useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { API, copy, showSuccess, showError, renderQuota } from '../../helpers';

const containerStyle = {
  background:
    'linear-gradient(135deg, var(--semi-color-success-light-default), var(--semi-color-success-light-hover))',
  borderRadius: 16,
  padding: '16px 14px',
  transition: 'box-shadow 200ms ease',
  position: 'relative',
  height: '100%',
  display: 'flex',
  flexDirection: 'column',
};

const kpiCellStyle = {
  background: 'rgba(255,255,255,0.55)',
  borderRadius: 10,
  padding: '8px 6px',
  textAlign: 'center',
};

const AffiliateRewardCard = ({ summary, loading }) => {
  const { t } = useTranslation();
  const navigate = useNavigate();

  // Lazy-load aff link only on user intent (copy button click).
  // The /api/user/aff endpoint auto-generates a code on first call (write
  // side-effect), so eager-fetching on dashboard mount would silently create
  // aff codes for every visiting user.
  const affLinkRef = useRef('');
  const [copying, setCopying] = useState(false);

  const ensureAffLink = async () => {
    if (affLinkRef.current) return affLinkRef.current;
    const res = await API.get('/api/user/aff');
    if (res?.data?.success && res.data.data) {
      affLinkRef.current = `${window.location.origin}/register?aff=${res.data.data}`;
      return affLinkRef.current;
    }
    return '';
  };

  if (loading) {
    return (
      <div style={containerStyle} className='hover:shadow-lg'>
        <Skeleton
          loading={true}
          active
          placeholder={
            <div className='flex flex-col gap-3'>
              <Skeleton.Title style={{ width: '60%', height: 16 }} />
              <Skeleton.Paragraph rows={2} />
              <Skeleton.Paragraph rows={2} />
              <Skeleton.Title style={{ width: '100%', height: 32 }} />
            </div>
          }
        />
      </div>
    );
  }

  if (!summary) {
    return null;
  }

  const isFrozen = summary.aff_status === 'frozen';
  const isEmpty =
    !isFrozen &&
    (summary.aff_count || 0) === 0 &&
    (summary.aff_history_quota || 0) === 0;

  const pendingDisplay = `$${(summary.pending_amount_usd || 0).toFixed(2)}`;
  const monthDisplay = `$${(summary.this_month_earned_usd || 0).toFixed(2)}`;
  const lifetimeDisplay = renderQuota(summary.aff_history_quota || 0);
  const inviteCount = summary.aff_count || 0;
  const rewardPct = summary.reward_percent ?? 10;

  const handleCopy = async () => {
    if (copying || isFrozen) return;
    setCopying(true);
    try {
      const link = await ensureAffLink();
      if (!link) {
        showError(t('复制失败，请手动复制'));
        return;
      }
      const ok = await copy(link);
      if (ok) {
        showSuccess(t('邀请链接已复制到剪切板'));
      } else {
        showError(t('复制失败，请手动复制'));
      }
    } catch (e) {
      showError(t('复制失败，请手动复制'));
    } finally {
      setCopying(false);
    }
  };

  const ctaDisabled = isFrozen;
  const ctaText = isEmpty
    ? t('立即邀请第一位好友')
    : t('复制邀请链接');

  return (
    <div style={containerStyle} className='hover:shadow-lg'>
      {/* 1. Title row */}
      <div className='flex items-center justify-between'>
        <div className='flex items-center gap-1.5'>
          <Gift size={16} className='text-semi-color-success' />
          <span className='text-sm font-bold text-semi-color-text-0'>
            {t('邀请奖励')}
          </span>
        </div>
        <Tag color='green' size='small' shape='circle'>
          {t('{{p}}% 返佣', { p: rewardPct })}
        </Tag>
      </div>

      {/* 2. Headline copy or frozen banner */}
      {isFrozen ? (
        <div className='mt-3'>
          <Banner
            type='warning'
            fullMode={false}
            closeIcon={null}
            description={t('您的分销资格已冻结，请联系客服')}
          />
        </div>
      ) : (
        <div className='mt-3'>
          <div className='text-[13px] font-bold leading-snug text-semi-color-text-0'>
            {t('让 token 不再有预算')}
          </div>
          <div className='text-[11px] mt-1 text-semi-color-text-2 leading-snug'>
            {isEmpty
              ? t('从邀请第一位好友开始')
              : t('好友充值/月卡续费，每次都拿返佣')}
          </div>
        </div>
      )}

      {/* 3. Dual KPI cards */}
      <div className='grid grid-cols-2 gap-2 mt-3'>
        <div style={kpiCellStyle}>
          <div className='text-lg font-extrabold text-semi-color-text-0 leading-tight'>
            {inviteCount}
          </div>
          <div className='text-[10px] mt-0.5 text-semi-color-text-2'>
            {t('邀请人数')}
          </div>
        </div>
        <div style={kpiCellStyle}>
          <div className='text-lg font-extrabold text-semi-color-text-0 leading-tight'>
            {lifetimeDisplay}
          </div>
          <div className='text-[10px] mt-0.5 text-semi-color-text-2'>
            {t('累计返佣')}
          </div>
        </div>
      </div>

      {/* 4. KV detail rows */}
      <div className='mt-3 space-y-1'>
        <div className='flex justify-between items-baseline text-[11px]'>
          <span className='text-semi-color-text-2'>{t('冷却中')}</span>
          <span className='font-bold text-semi-color-text-0'>
            {pendingDisplay}
          </span>
        </div>
        <div className='flex justify-between items-baseline text-[11px]'>
          <span className='text-semi-color-text-2'>{t('本月新增')}</span>
          <span className='font-bold text-semi-color-text-0'>
            {monthDisplay}
          </span>
        </div>
      </div>

      {/* 5. Primary CTA */}
      <Button
        theme='solid'
        type='primary'
        block
        disabled={ctaDisabled}
        loading={copying}
        onClick={handleCopy}
        icon={<Copy size={14} />}
        className='!mt-3'
      >
        {ctaText}
      </Button>

      {/* 6. Secondary link */}
      <div
        className='text-center text-[11px] mt-2 cursor-pointer text-semi-color-text-2 hover:text-semi-color-primary'
        onClick={() => navigate('/console/topup')}
      >
        {t('查看返佣明细')} →
      </div>
    </div>
  );
};

export default AffiliateRewardCard;
