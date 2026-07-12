import assert from 'node:assert/strict';
import test from 'node:test';

import {
  canLockPlan,
  canSetCurrent,
  enrichPlansWithQueueMetadata,
  getInactiveKind,
  groupPlans,
  isPlansRouteEnabled,
  isUserLocked,
  planDisplayName,
  planTypeKey,
  quotaSummary,
} from './utils.js';

const now = Date.UTC(2026, 6, 12);
const plan = (id, fields = {}) => ({
  id,
  status: 1,
  is_current: 0,
  locked: 0,
  queue_position: 0,
  started_at: now - 1000,
  expires_at: now + 86400000,
  plan_priority: 10,
  quota: 80,
  used_quota: 20,
  ...fields,
});

test('groups by current, inactive, locked, queued, then available precedence', () => {
  const groups = groupPlans(
    [
      plan(1, { is_current: 1, locked: 1 }),
      plan(2, { status: 2, expires_at: now - 1000 }),
      plan(3, { status: 1, expires_at: now }),
      plan(4, { locked: 1, queue_position: 2, started_at: 0 }),
      plan(5, { queue_position: 1, started_at: 0 }),
      plan(6),
    ],
    now,
  );
  assert.equal(groups.current.id, 1);
  assert.deepEqual(
    groups.inactive.map((item) => item.id),
    [3, 2],
  );
  assert.deepEqual(
    groups.locked.map((item) => item.id),
    [4],
  );
  assert.deepEqual(
    groups.queued.map((item) => item.id),
    [5],
  );
  assert.deepEqual(
    groups.available.map((item) => item.id),
    [6],
  );
  assert.equal(getInactiveKind(plan(7, { status: 4 }), now), 'completed');
  assert.equal(
    getInactiveKind(plan(8, { status: 1, expires_at: now }), now),
    'expired',
  );
  assert.equal(
    isUserLocked(plan(9, { locked: 1, locked_by: 'user' })),
    true,
  );
  assert.equal(
    isUserLocked(plan(10, { locked: 1, locked_by: 'admin' })),
    false,
  );
});

test('sorts available and locked by priority desc then id asc', () => {
  const groups = groupPlans(
    [
      plan(9, { plan_priority: 5 }),
      plan(8, { plan_priority: 20 }),
      plan(7, { plan_priority: 20 }),
      plan(6, { locked: 1, plan_priority: 3 }),
      plan(5, { locked: 1, plan_priority: 9 }),
    ],
    now,
  );
  assert.deepEqual(
    groups.available.map((item) => item.id),
    [7, 8, 9],
  );
  assert.deepEqual(
    groups.locked.map((item) => item.id),
    [5, 6],
  );
});

test('sorts queued by position and inactive by recent expiry with zero last', () => {
  const groups = groupPlans(
    [
      plan(1, { queue_position: 3, started_at: 0 }),
      plan(2, { queue_position: 1, started_at: 0 }),
      plan(3, { status: 4, expires_at: now - 2000 }),
      plan(4, { status: 5, expires_at: now - 1000 }),
      plan(5, { status: 6, expires_at: 0 }),
      plan(6, { status: 2, expires_at: now - 3000 }),
      plan(7, { status: 2, expires_at: now - 3000 }),
    ],
    now,
  );
  assert.deepEqual(
    groups.queued.map((item) => item.id),
    [2, 1],
  );
  assert.deepEqual(
    groups.inactive.map((item) => item.id),
    [4, 3, 7, 6, 5],
  );
});

test('joins estimated activation by user plan id without mutating source', () => {
  const source = [
    plan(1),
    plan(2, { locked: 1, queue_position: 2, started_at: 0 }),
  ];
  const result = enrichPlansWithQueueMetadata(source, [
    { id: 2, estimated_activation_time: now + 5000 },
  ]);
  assert.equal(result[1].estimated_activation_time, now + 5000);
  assert.equal(source[1].estimated_activation_time, undefined);
});

test('action predicates match backend atomic eligibility', () => {
  assert.equal(canSetCurrent(plan(1), now), true);
  assert.equal(canSetCurrent(plan(1, { can_switch: 0 }), now), true);
  assert.equal(
    canSetCurrent(plan(1, { queue_position: 1, started_at: 0 }), now),
    false,
  );
  assert.equal(
    canSetCurrent(plan(1, { queue_position: 1, started_at: now - 1000 }), now),
    false,
  );
  assert.equal(canSetCurrent(plan(1, { quota: 0 }), now), false);
  assert.equal(canSetCurrent(plan(1, { locked: 1 }), now), false);
  assert.equal(canSetCurrent(plan(1, { expires_at: now }), now), false);
  assert.equal(canLockPlan(plan(1), now), true);
  assert.equal(canLockPlan(plan(1, { quota: 0 }), now), true);
  assert.equal(canLockPlan(plan(1, { is_current: 1 }), now), false);
  assert.equal(canLockPlan(plan(1, { status: 3 }), now), false);
});

test('quota summary clamps malformed values and display/type labels follow DTO fallbacks', () => {
  assert.deepEqual(quotaSummary(plan(1)), {
    total: 100,
    used: 20,
    remaining: 80,
    remainingPercent: 80,
  });
  assert.deepEqual(quotaSummary(plan(2, { quota: -5, used_quota: 0 })), {
    total: 0,
    used: 0,
    remaining: 0,
    remainingPercent: 0,
  });
  assert.equal(
    planDisplayName({
      plan_display_name: 'Snapshot',
      plan: { display_name: 'Template' },
    }),
    'Snapshot',
  );
  assert.equal(planDisplayName({ plan_name: 'name' }), 'name');
  assert.equal(planTypeKey({ plan_type: 'subscription' }), '订阅套餐');
  assert.equal(planTypeKey({ plan: { type: 'trial' } }), '试用套餐');
  assert.equal(planTypeKey({ plan_type: 'custom' }), '未知类型');
});

test('plans route visibility mirrors the application router contract', () => {
  assert.equal(isPlansRouteEnabled('{"plans":false}', true), false);
  assert.equal(isPlansRouteEnabled('{"plans":true}', true), true);
  assert.equal(isPlansRouteEnabled('invalid-json', true), true);
  assert.equal(isPlansRouteEnabled('', true), true);
  assert.equal(isPlansRouteEnabled('', false), false);
});
