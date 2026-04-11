/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React, { useContext, useEffect, useState } from 'react';
import { Button, Card, Form, Space, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';
import { StatusContext } from '../../../context/Status';

const DEFAULT_TEMPLATE = [
  {
    key: 'gemini-2.5-flash-image',
    label: 'Gemini 2.5 Flash Image',
    apiType: 'chat',
    supportRefImage: true,
    maxRefImages: 4,
    sizes: ['1024x1024', '1024x1792', '1792x1024'],
    defaultSize: '1024x1024',
    batchEnabled: true,
  },
  {
    key: 'gpt-image-1',
    label: 'GPT Image 1',
    apiType: 'images',
    supportRefImage: false,
    maxRefImages: 0,
    sizes: ['1024x1024', '1024x1536', '1536x1024'],
    defaultSize: '1024x1024',
    batchEnabled: false,
  },
];

const REQUIRED_FIELDS = [
  'key',
  'label',
  'apiType',
  'sizes',
  'defaultSize',
];

function validate(list) {
  if (!Array.isArray(list)) return 'must be an array';
  for (const m of list) {
    for (const f of REQUIRED_FIELDS) {
      if (m[f] === undefined) return f;
    }
    if (m.apiType !== 'chat' && m.apiType !== 'images') return 'apiType';
    if (!Array.isArray(m.sizes) || m.sizes.length === 0) return 'sizes';
  }
  return null;
}

export default function SettingsDrawFactoryModels(props) {
  const { t } = useTranslation();
  const [text, setText] = useState('');
  const [loading, setLoading] = useState(false);
  const [statusState, statusDispatch] = useContext(StatusContext);

  useEffect(() => {
    const raw = props.options?.DrawFactoryModels;
    if (raw) {
      try {
        setText(JSON.stringify(JSON.parse(raw), null, 2));
      } catch (_e) {
        setText(raw);
      }
    } else {
      setText(JSON.stringify(DEFAULT_TEMPLATE, null, 2));
    }
  }, [props.options]);

  async function onSubmit() {
    let parsed;
    try {
      parsed = JSON.parse(text);
    } catch (_e) {
      showError(t('draw_factory.admin.invalid_json'));
      return;
    }
    const missing = validate(parsed);
    if (missing) {
      showError(t('draw_factory.admin.missing_field', { field: missing }));
      return;
    }
    setLoading(true);
    try {
      const value = JSON.stringify(parsed);
      const res = await API.put('/api/option/', {
        key: 'DrawFactoryModels',
        value,
      });
      if (res.data.success) {
        showSuccess(t('draw_factory.admin.save'));
        statusDispatch({
          type: 'set',
          payload: {
            ...statusState.status,
            DrawFactoryModels: value,
          },
        });
        if (props.refresh) await props.refresh();
      } else {
        showError(res.data.message);
      }
    } catch (_e) {
      showError('save failed');
    } finally {
      setLoading(false);
    }
  }

  function resetDefault() {
    setText(JSON.stringify(DEFAULT_TEMPLATE, null, 2));
  }

  return (
    <Card>
      <Form.Section text={t('draw_factory.admin.section_title')}>
        <Typography.Text>{t('draw_factory.admin.models_label')}</Typography.Text>
        <textarea
          value={text}
          onChange={(e) => setText(e.target.value)}
          rows={18}
          style={{
            width: '100%',
            fontFamily: 'monospace',
            fontSize: 13,
            marginTop: 8,
            padding: 12,
            borderRadius: 8,
            border: '1px solid var(--semi-color-border)',
            background: 'var(--semi-color-bg-1)',
            color: 'var(--semi-color-text-0)',
          }}
        />
        <Space style={{ marginTop: 12 }}>
          <Button onClick={resetDefault}>
            {t('draw_factory.admin.reset_default')}
          </Button>
          <Button theme='solid' type='primary' loading={loading} onClick={onSubmit}>
            {t('draw_factory.admin.save')}
          </Button>
        </Space>
      </Form.Section>
    </Card>
  );
}
