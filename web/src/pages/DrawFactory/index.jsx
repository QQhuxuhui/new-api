/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
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
