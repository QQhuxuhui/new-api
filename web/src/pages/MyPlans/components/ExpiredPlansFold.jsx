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

import { Button, Tag } from '@douyinfe/semi-ui';
import { ChevronDown, ChevronUp } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import CompactPlanCard from './CompactPlanCard';
import { getInactiveKind } from '../utils';

const labels = {
  expired: '已过期',
  disabled: '已停用',
  completed: '已用完',
  forfeited: '已作废',
  revoked: '已回收',
};

const ExpiredPlansFold = ({ plans, onOpenDetails }) => {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const counts = useMemo(
    () =>
      plans.reduce((result, plan) => {
        const kind = getInactiveKind(plan);
        if (kind) result[kind] = (result[kind] || 0) + 1;
        return result;
      }, {}),
    [plans],
  );
  if (!plans.length) return null;

  return (
    <section aria-labelledby='inactive-plans-title'>
      <Button
        block
        theme='light'
        className='!h-auto !rounded-lg !px-4 !py-3'
        aria-expanded={expanded}
        aria-controls='inactive-plans-grid'
        onClick={() => setExpanded((value) => !value)}
      >
        <span className='flex w-full flex-wrap items-center gap-2 text-left'>
          <span
            id='inactive-plans-title'
            className='mr-auto font-semibold text-semi-color-text-0'
          >
            {t('已失效')}
          </span>
          {Object.entries(labels).map(([kind, label]) =>
            counts[kind] ? (
              <Tag key={kind}>
                {t(label)} {counts[kind]}
              </Tag>
            ) : null,
          )}
          {expanded ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
        </span>
      </Button>
      {expanded && (
        <div
          id='inactive-plans-grid'
          className='mt-4 grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3'
        >
          {plans.map((plan) => (
            <CompactPlanCard
              key={plan.id}
              plan={plan}
              section='inactive'
              pendingAction={null}
              onSwitch={() => undefined}
              onLock={() => undefined}
              onUnlock={() => undefined}
              onOpenDetails={onOpenDetails}
            />
          ))}
        </div>
      )}
    </section>
  );
};

export default ExpiredPlansFold;
