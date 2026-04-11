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
import { SideSheet, Button, Empty } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';
import { getHistory, clearHistory } from '../../helpers/drawFactoryStorage';

export default function HistoryDrawer({ visible, onClose, refreshKey }) {
  const { t } = useTranslation();
  // refreshKey lets parent force re-read after a new generation
  const list = React.useMemo(() => getHistory(), [refreshKey, visible]);

  return (
    <SideSheet
      title={t('draw_factory.action.history')}
      visible={visible}
      onCancel={onClose}
      width={420}
    >
      <Button
        onClick={() => {
          clearHistory();
          onClose();
        }}
        style={{ marginBottom: 12 }}
      >
        {t('draw_factory.action.clear_history')}
      </Button>
      {list.length === 0 ? (
        <Empty />
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          {list.map((item) => (
            <div
              key={item.id}
              style={{
                border: '1px solid var(--semi-color-border)',
                borderRadius: 8,
                padding: 12,
              }}
            >
              <div style={{ fontSize: 12, color: 'var(--semi-color-text-2)' }}>
                {item.model} · {new Date(item.createdAt).toLocaleString()}
              </div>
              <div
                style={{
                  fontSize: 13,
                  margin: '6px 0',
                  fontStyle: 'italic',
                }}
              >
                {item.prompt}
              </div>
              {item.image && (
                <img
                  src={item.image}
                  alt='history'
                  style={{
                    width: '100%',
                    borderRadius: 6,
                    border: '1px solid var(--semi-color-border)',
                  }}
                />
              )}
              {item.error && (
                <div
                  style={{
                    color: 'var(--semi-color-danger)',
                    fontSize: 13,
                  }}
                >
                  {item.error}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </SideSheet>
  );
}
