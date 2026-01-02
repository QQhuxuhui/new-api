import { API } from './api';

export async function getDisableRules() {
  return API.get('/api/channel/disable-rules/');
}

export async function createDisableRule(rule) {
  return API.post('/api/channel/disable-rules/', rule);
}

export async function updateDisableRule(id, rule) {
  return API.put(`/api/channel/disable-rules/${id}`, rule);
}

export async function deleteDisableRule(id) {
  return API.delete(`/api/channel/disable-rules/${id}`);
}

export async function testDisableRules(payload) {
  return API.post('/api/channel/disable-rules/test', payload);
}
