/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React from 'react';
import { Button } from '@douyinfe/semi-ui';

export default function SizePicker({ sizes, value, onChange }) {
  if (!sizes || sizes.length === 0) return null;
  return (
    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
      {sizes.map((s) => (
        <Button
          key={s}
          size='small'
          type={s === value ? 'primary' : 'tertiary'}
          theme={s === value ? 'solid' : 'light'}
          onClick={() => onChange(s)}
        >
          {s}
        </Button>
      ))}
    </div>
  );
}
