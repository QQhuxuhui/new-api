/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React from 'react';
import { TextArea } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

export default function PromptInput({ value, onChange }) {
  const { t } = useTranslation();
  return (
    <TextArea
      value={value}
      onChange={onChange}
      autosize={{ minRows: 2, maxRows: 6 }}
      placeholder={t('draw_factory.field.prompt_placeholder')}
    />
  );
}
