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
import { Tabs, TabPane, Empty } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import Forbidden from '../Forbidden';
import { useDrawFactoryConfig } from '../../hooks/drawFactory/useDrawFactoryConfig';
import SinglePanel from './SinglePanel';
import BatchPanel from './BatchPanel';

export default function DrawFactory() {
  const { t } = useTranslation();
  const { enabled, models } = useDrawFactoryConfig();

  if (!enabled) return <Forbidden />;

  if (models.length === 0) {
    return (
      <div style={{ padding: 24 }}>
        <Empty
          title={t('draw_factory.title')}
          description={t('draw_factory.empty.no_models')}
        />
      </div>
    );
  }

  return (
    <div style={{ padding: 24, maxWidth: 1000, margin: '0 auto' }}>
      <Tabs type='line'>
        <TabPane tab={t('draw_factory.tab.single')} itemKey='single'>
          <SinglePanel models={models} />
        </TabPane>
        <TabPane tab={t('draw_factory.tab.batch')} itemKey='batch'>
          <BatchPanel models={models} />
        </TabPane>
      </Tabs>
    </div>
  );
}
