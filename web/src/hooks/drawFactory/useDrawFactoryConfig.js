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
