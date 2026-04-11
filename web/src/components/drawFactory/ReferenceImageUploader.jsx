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

import React, { useRef } from 'react';
import { Button, Toast } from '@douyinfe/semi-ui';
import { useTranslation } from 'react-i18next';

const MAX_BYTES = 10 * 1024 * 1024;

function readAsDataUrl(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result);
    reader.onerror = reject;
    reader.readAsDataURL(file);
  });
}

export default function ReferenceImageUploader({ refs, onChange, max = 4 }) {
  const { t } = useTranslation();
  const inputRef = useRef(null);

  async function handleFiles(fileList) {
    const files = Array.from(fileList || []);
    const remaining = max - refs.length;
    if (remaining <= 0) {
      Toast.warning(t('draw_factory.error.too_many_refs'));
      return;
    }
    const next = [...refs];
    for (const file of files.slice(0, remaining)) {
      if (file.size > MAX_BYTES) {
        Toast.warning(t('draw_factory.error.ref_too_large'));
        continue;
      }
      const url = await readAsDataUrl(file);
      next.push(url);
    }
    onChange(next);
  }

  function remove(idx) {
    const next = refs.slice();
    next.splice(idx, 1);
    onChange(next);
  }

  return (
    <div>
      <input
        ref={inputRef}
        type='file'
        accept='image/*'
        multiple
        style={{ display: 'none' }}
        onChange={(e) => {
          handleFiles(e.target.files);
          e.target.value = '';
        }}
      />
      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginBottom: 8 }}>
        {refs.map((url, i) => (
          <div
            key={i}
            style={{
              position: 'relative',
              width: 80,
              height: 80,
              borderRadius: 8,
              overflow: 'hidden',
              border: '2px solid var(--semi-color-border)',
            }}
          >
            <img
              src={url}
              alt={`ref-${i}`}
              style={{ width: '100%', height: '100%', objectFit: 'cover' }}
            />
            <Button
              size='small'
              style={{
                position: 'absolute',
                top: 2,
                right: 2,
              }}
              onClick={() => remove(i)}
              aria-label='remove'
            >
              ×
            </Button>
          </div>
        ))}
      </div>
      <Button
        onClick={() => inputRef.current?.click()}
        disabled={refs.length >= max}
      >
        {t('draw_factory.field.reference_images')} ({refs.length}/{max})
      </Button>
    </div>
  );
}
