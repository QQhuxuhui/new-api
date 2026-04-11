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

import React, { useEffect, useState } from 'react';
import { Select, Spin } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { API, showError } from '../../helpers';

export default function TokenSelector({ value, onChange }) {
  const { t } = useTranslation();
  const [tokens, setTokens] = useState([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let mounted = true;
    setLoading(true);
    // TUNE-POINT 1: /api/token/?p=1&size=100 matches actual usage (1-based page)
    API.get('/api/token/?p=1&size=100')
      .then((res) => {
        if (!mounted) return;
        // TUNE-POINT 2: response is res.data.data.items (paginated payload object)
        const list = res?.data?.data?.items || [];
        const active = Array.isArray(list)
          ? list.filter((tk) => tk.status === 1)
          : [];
        setTokens(active);
        if (!value && active.length > 0) {
          onChange(active[0]);
        }
      })
      .catch((e) => showError(e?.message || 'failed'))
      .finally(() => mounted && setLoading(false));
    return () => {
      mounted = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (loading) return <Spin />;

  return (
    <Select
      style={{ width: '100%' }}
      placeholder={t('draw_factory.field.token')}
      value={value?.id}
      onChange={(id) => onChange(tokens.find((tk) => tk.id === id))}
      optionList={tokens.map((tk) => ({
        label: `${tk.name} (${String(tk.key || '').slice(0, 8)}…)`,
        value: tk.id,
      }))}
      emptyContent={t('draw_factory.empty.no_tokens')}
    />
  );
}
