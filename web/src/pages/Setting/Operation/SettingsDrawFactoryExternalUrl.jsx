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

// Admin setting for the external URL the sidebar "绘图工厂" entry redirects to.
// Empty means "hide the menu item"; non-empty means open in a new tab on click.
export default function SettingsDrawFactoryExternalUrl(props) {
  const { t } = useTranslation();
  const [value, setValue] = useState('');
  const [loading, setLoading] = useState(false);
  const [statusState, statusDispatch] = useContext(StatusContext);

  useEffect(() => {
    setValue(props.options?.DrawFactoryExternalUrl || '');
  }, [props.options]);

  function normalize(v) {
    return (v || '').trim();
  }

  function isValid(v) {
    if (v === '') return true;
    return /^https?:\/\/[^\s]+$/i.test(v);
  }

  async function onSubmit() {
    const normalized = normalize(value);
    if (!isValid(normalized)) {
      showError(t('请填写合法的 http(s) URL'));
      return;
    }
    setLoading(true);
    try {
      const res = await API.put('/api/option/', {
        key: 'DrawFactoryExternalUrl',
        value: normalized,
      });
      if (res.data.success) {
        showSuccess(t('保存成功'));
        statusDispatch({
          type: 'set',
          payload: {
            ...statusState.status,
            DrawFactoryExternalUrl: normalized,
          },
        });
        setValue(normalized);
        if (props.refresh) await props.refresh();
      } else {
        showError(res.data.message);
      }
    } catch (_e) {
      showError(t('保存失败'));
    } finally {
      setLoading(false);
    }
  }

  return (
    <Card>
      <Form.Section text={t('绘图工厂菜单跳转链接')}>
        <Typography.Text>
          {t('外部绘图平台 URL')}
        </Typography.Text>
        <Input
          value={value}
          onChange={setValue}
          placeholder='https://your-drawing-platform.example.com'
          style={{ marginTop: 8 }}
        />
        <Typography.Text type='tertiary' style={{ display: 'block', marginTop: 8 }}>
          {t(
            '配置后，侧边栏“绘图工厂”菜单点击会在新标签页打开该地址；留空则不显示该菜单项。',
          )}
        </Typography.Text>
        <Space style={{ marginTop: 12 }}>
          <Button theme='solid' type='primary' loading={loading} onClick={onSubmit}>
            {t('保存')}
          </Button>
        </Space>
      </Form.Section>
    </Card>
  );
}
