/*
Copyright (C) 2025 QuantumNous

Inviter reward (offline payout ledger) API client.
*/

import { API, showError } from '../helpers';

const BASE = '/api/user/manage';

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
      });
      if (res.data.success) return res.data.data;
      throw new Error(res.data.message || 'Failed to fetch invitee recharges');
    } catch (err) {
      showError(err.message || 'Failed to fetch invitee recharges');
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
      });
      if (res.data.success) return res.data.data;
      throw new Error(res.data.message || 'Failed to fetch payout history');
    } catch (err) {
      showError(err.message || 'Failed to fetch payout history');
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
      const res = await API.post(`${BASE}/${inviterId}/inviter-reward-payouts`, body);
      if (res.data.success) return res.data.data;
      throw new Error(res.data.message || 'Failed to create payout');
    } catch (err) {
      showError(err.message || 'Failed to create payout');
      throw err;
    }
  },
};

export default InviterRewardAPI;
