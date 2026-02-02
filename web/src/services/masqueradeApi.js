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

import { API, showError } from '../helpers';

const BASE_URL = '/api/masquerade';

export const MasqueradeAPI = {
  async fetchTraces() {
    try {
      const response = await API.get(`${BASE_URL}/traces`);
      if (response.data.success) {
        return response.data.data || [];
      }
      throw new Error(response.data.message || 'Failed to fetch masquerade traces');
    } catch (error) {
      showError(error.message || 'Failed to fetch masquerade traces');
      throw error;
    }
  },

  async clearTraces() {
    try {
      const response = await API.post(`${BASE_URL}/clear`);
      if (response.data.success) {
        return response.data;
      }
      throw new Error(response.data.message || 'Failed to clear masquerade traces');
    } catch (error) {
      showError(error.message || 'Failed to clear masquerade traces');
      throw error;
    }
  },
};

