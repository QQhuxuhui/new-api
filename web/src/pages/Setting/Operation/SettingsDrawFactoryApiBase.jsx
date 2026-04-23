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

import React, { useContext, useEffect, useState } from 'react';
import { Button, Card, Form, Input, Space, Typography } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError, showSuccess } from '../../../helpers';
import { StatusContext } from '../../../context/Status';

// Admin setting for the API base URL used by DrawFactory (single + batch).
// Empty means "same origin as the web UI" (pre-config behavior).
export default function SettingsDrawFactoryApiBase(props) {
  const { t } = useTranslation();
  const [value, setValue] = useState('');
  const [loading, setLoading] = useState(false);
  const [statusState, statusDispatch] = useContext(StatusContext);

  useEffect(() => {
    setValue(props.options?.DrawFactoryApiBase || '');
  }, [props.options]);

  function normalize(v) {
    return (v || '').trim().replace(/\/+$/, '');
  }

  function isValid(v) {
    if (v === '') return true; // empty = fallback to same-origin
    return /^https?:\/\/[^\s/]+(:\d+)?(\/.*)?$/i.test(v);
  }

  async function onSubmit() {
    const normalized = normalize(value);
    if (!isValid(normalized)) {
      showError(t('draw_factory.admin.api_base_invalid'));
      return;
    }
    setLoading(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'DrawFactoryApiBase',
        value: normalized,
      });
      if (res.data.success) {
        showSuccess(t('draw_factory.admin.save'));
        statusDispatch({
          type: 'set',
          payload: {
            ...statusState.status,
            DrawFactoryApiBase: normalized,
          },
        });
        setValue(normalized);
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

  return (
    <Card>
      <Form.Section text={t('draw_factory.admin.api_base_section')}>
        <Typography.Text>
          {t('draw_factory.admin.api_base_label')}
        </Typography.Text>
        <Input
          value={value}
          onChange={setValue}
          placeholder='https://api.example.com'
          style={{ marginTop: 8 }}
        />
        <Typography.Text type='tertiary' style={{ display: 'block', marginTop: 8 }}>
          {t('draw_factory.admin.api_base_hint')}
        </Typography.Text>
        <Space style={{ marginTop: 12 }}>
          <Button theme='solid' type='primary' loading={loading} onClick={onSubmit}>
            {t('draw_factory.admin.save')}
          </Button>
        </Space>
      </Form.Section>
    </Card>
  );
}
