/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import React, { useContext, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Empty } from '@douyinfe/semi-ui';
import { StatusContext } from '../../context/Status';
import Forbidden from '../Forbidden';

function parseSidebarModules(raw) {
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch (_e) {
    return null;
  }
}

export default function DrawFactory() {
  const { t } = useTranslation();
  const [statusState] = useContext(StatusContext);

  const enabled = useMemo(() => {
    const modules = parseSidebarModules(
      statusState?.status?.SidebarModulesAdmin,
    );
    if (!modules) return true; // default on
    const chat = modules.chat;
    if (!chat || chat.enabled === false) return false;
    return chat.drawFactory !== false;
  }, [statusState?.status?.SidebarModulesAdmin]);

  if (!enabled) {
    return <Forbidden />;
  }

  return (
    <div style={{ padding: 24 }}>
      <Empty title={t('draw_factory.title')} description='coming soon' />
    </div>
  );
}
