/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
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
