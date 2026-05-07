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

const BASE = '/api/user/manage';

const getApiErrorMessage = (err, fallback) =>
  err?.response?.data?.message || err?.message || fallback;

export const InviterRewardAPI = {
  /**
   * Fetch invitee recharge summary + paginated detail rows for inviter user.
   * @param {number} inviterId
   * @param {number} page
   * @param {number} pageSize
   * @returns {Promise<{summary, items, pagination, default_percent}>}
   */
  async fetchInviteeRecharges(inviterId, page = 1, pageSize = 20) {
    try {
      const res = await API.get(`${BASE}/${inviterId}/invitee-recharges`, {
        params: { page, page_size: pageSize },
        skipErrorHandler: true,
      });
      if (res.data.success) return res.data.data;
      throw new Error(res.data.message || 'Failed to fetch invitee recharges');
    } catch (err) {
      showError(getApiErrorMessage(err, 'Failed to fetch invitee recharges'));
      throw err;
    }
  },

  /**
   * Fetch payout history for inviter user.
   */
  async fetchPayoutHistory(inviterId, page = 1, pageSize = 20) {
    try {
      const res = await API.get(`${BASE}/${inviterId}/inviter-reward-payouts`, {
        params: { page, page_size: pageSize },
        skipErrorHandler: true,
      });
      if (res.data.success) return res.data.data;
      throw new Error(res.data.message || 'Failed to fetch payout history');
    } catch (err) {
      showError(getApiErrorMessage(err, 'Failed to fetch payout history'));
      throw err;
    }
  },

  /**
   * Create a new payout batch (mark current pending recharges as rewarded).
   * @param {number} inviterId
   * @param {{payout_amount_usd: number, note?: string}} body
   */
  async createPayout(inviterId, body) {
    try {
      const res = await API.post(`${BASE}/${inviterId}/inviter-reward-payouts`, body, {
        skipErrorHandler: true,
      });
      if (res.data.success) return res.data.data;
      throw new Error(res.data.message || 'Failed to create payout');
    } catch (err) {
      showError(getApiErrorMessage(err, 'Failed to create payout'));
      throw err;
    }
  },
};

export default InviterRewardAPI;
