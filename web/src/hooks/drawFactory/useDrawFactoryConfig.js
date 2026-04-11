/*
Copyright (C) 2025 QuantumNous
SPDX-License-Identifier: AGPL-3.0-or-later
*/

import { useContext, useMemo } from 'react';
import { StatusContext } from '../../context/Status';

function parseSidebarModules(raw) {
  if (!raw) return null;
  try {
    return JSON.parse(raw);
  } catch (_e) {
    return null;
  }
}

function parseModels(raw) {
  if (!raw) return [];
  try {
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch (_e) {
    return [];
  }
}

// Validates a model whitelist entry. Returns true if usable.
function isValidModel(m) {
  return (
    m &&
    typeof m.key === 'string' &&
    typeof m.label === 'string' &&
    (m.apiType === 'chat' || m.apiType === 'images') &&
    Array.isArray(m.sizes) &&
    typeof m.defaultSize === 'string'
  );
}

export function useDrawFactoryConfig() {
  const [statusState] = useContext(StatusContext);
  const rawSidebar = statusState?.status?.SidebarModulesAdmin;
  const rawModels = statusState?.status?.DrawFactoryModels;

  const enabled = useMemo(() => {
    const modules = parseSidebarModules(rawSidebar);
    if (!modules) return true; // default on
    const chat = modules.chat;
    if (!chat || chat.enabled === false) return false;
    return chat.drawFactory !== false;
  }, [rawSidebar]);

  const models = useMemo(() => {
    return parseModels(rawModels).filter(isValidModel);
  }, [rawModels]);

  return { enabled, models };
}
