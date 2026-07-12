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

import { Progress, Tag, Tooltip, Typography } from '@douyinfe/semi-ui';
import { Moon, Zap } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { renderQuota } from '../../../helpers';

const { Text, Title } = Typography;

const DailyPoolCard = ({ pool }) => {
  const { t } = useTranslation();
  if (!pool) return null;

  const total = Math.max(0, Number(pool.total) || 0);
  const used = Math.max(0, Number(pool.used) || 0);
  const available = Math.max(0, Number(pool.available) || 0);
  const usedPercent =
    total > 0 ? Math.min(100, Math.max(0, (used / total) * 100)) : 0;
  const remainingPercent =
    total > 0 ? Math.min(100, Math.max(0, (available / total) * 100)) : 0;
  const hour = new Date().getHours();
  const isLateNight = hour >= 22 || hour < 6;

  return (
    <section className='rounded-lg border border-semi-color-border bg-semi-color-bg-1 p-4'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <div>
          <Title heading={5} className='m-0'>
            <Zap size={17} className='mr-1 inline' /> {t('今日日卡池')}
          </Title>
          <Text type='tertiary' size='small'>
            {t('有效期至')}: {pool.expires_at}
          </Text>
        </div>
        <div className='flex flex-wrap gap-2'>
          <Tag>
            {t('总额度')}: {renderQuota(total)}
          </Tag>
          <Tag color='orange'>
            {t('已使用')}: {renderQuota(used)}
          </Tag>
          <Tag color='green'>
            {t('剩余')}: {renderQuota(available)}
          </Tag>
          {isLateNight && (
            <Tooltip
              content={t(
                '当前为深夜时段，日卡将在明日凌晨重置，请合理安排使用',
              )}
            >
              <Tag color='orange'>
                <Moon size={14} /> {t('深夜提醒')}
              </Tag>
            </Tooltip>
          )}
        </div>
      </div>
      <Progress className='mt-3' percent={remainingPercent} showInfo={false} />
      <div className='mt-1 flex justify-between gap-3'>
        <Text type='tertiary' size='small'>
          {t('使用进度')}: {usedPercent.toFixed(1)}%
        </Text>
        <Text size='small'>
          {t('剩余')}: {remainingPercent.toFixed(1)}%
        </Text>
      </div>
    </section>
  );
};

export default DailyPoolCard;
