/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React from 'react';
import { Select } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

export default function ModelSelector({
  models,
  value,
  onChange,
  filter, // optional (m) => boolean — e.g. m.batchEnabled for batch tab
}) {
  const { t } = useTranslation();
  const list = filter ? models.filter(filter) : models;

  return (
    <Select
      style={{ width: '100%' }}
      placeholder={t('draw_factory.field.model')}
      value={value}
      onChange={onChange}
      optionList={list.map((m) => ({ label: m.label, value: m.key }))}
      emptyContent={t('draw_factory.empty.no_models')}
    />
  );
}
