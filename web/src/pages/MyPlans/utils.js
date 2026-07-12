const inactiveStatuses = new Set([2, 3, 4, 5, 6]);

const number = (value) => {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
};

const priority = (plan) => number(plan.plan_priority ?? plan.plan?.priority);
const isPseudoExpired = (plan, nowMs) =>
  number(plan.status) === 1 &&
  number(plan.expires_at) > 0 &&
  number(plan.expires_at) <= nowMs;

export const planDisplayName = (plan) =>
  plan?.plan_display_name ||
  plan?.plan_name ||
  plan?.plan?.display_name ||
  plan?.plan?.name ||
  '';

const planTypeKeys = {
  subscription: '订阅套餐',
  consumption: '按量付费',
  trial: '试用套餐',
  enterprise: '企业套餐',
};

export const planTypeKey = (plan) =>
  planTypeKeys[plan?.plan_type || plan?.plan?.type] || '未知类型';

export const isPlansRouteEnabled = (rawConfig, statusLoaded = true) => {
  if (!statusLoaded) return false;
  if (!rawConfig) return true;
  try {
    return JSON.parse(rawConfig)?.plans !== false;
  } catch {
    return true;
  }
};

export const getInactiveKind = (plan, nowMs = Date.now()) => {
  const status = number(plan.status);
  if (status === 2 || isPseudoExpired(plan, nowMs)) return 'expired';
  if (status === 3) return 'disabled';
  if (status === 4) return 'completed';
  if (status === 5) return 'forfeited';
  if (status === 6) return 'revoked';
  return null;
};

export const isQueuedPlan = (plan) =>
  number(plan.queue_position) > 0 && number(plan.started_at) === 0;

export const isUserLocked = (plan) =>
  number(plan.locked) === 1 && plan.locked_by === 'user';

export const canSetCurrent = (plan, nowMs = Date.now()) =>
  number(plan.is_current) !== 1 &&
  number(plan.locked) !== 1 &&
  number(plan.queue_position) === 0 &&
  number(plan.quota) > 0 &&
  number(plan.status) === 1 &&
  !isPseudoExpired(plan, nowMs);

export const canLockPlan = (plan, nowMs = Date.now()) =>
  number(plan.is_current) !== 1 &&
  number(plan.locked) !== 1 &&
  number(plan.queue_position) === 0 &&
  number(plan.status) === 1 &&
  !isPseudoExpired(plan, nowMs);

export const quotaSummary = (plan) => {
  const remaining = Math.max(0, number(plan.quota));
  const used = Math.max(0, number(plan.used_quota));
  const total = remaining + used;
  const remainingPercent =
    total === 0
      ? 0
      : Math.min(100, Math.max(0, (remaining / total) * 100));
  return { total, used, remaining, remainingPercent };
};

export const enrichPlansWithQueueMetadata = (plans = [], queuedPlans = []) => {
  const byId = new Map(
    queuedPlans.map((plan) => [number(plan.id), plan]),
  );
  return plans.map((plan) => {
    const queued = byId.get(number(plan.id));
    return queued
      ? {
          ...plan,
          estimated_activation_time: number(queued.estimated_activation_time),
        }
      : { ...plan };
  });
};

export const groupPlans = (plans = [], nowMs = Date.now()) => {
  const groups = {
    current: null,
    available: [],
    queued: [],
    locked: [],
    inactive: [],
  };
  for (const plan of plans) {
    if (number(plan.is_current) === 1 && groups.current === null) {
      groups.current = plan;
    } else if (
      inactiveStatuses.has(number(plan.status)) ||
      isPseudoExpired(plan, nowMs)
    ) {
      groups.inactive.push(plan);
    } else if (number(plan.locked) === 1) {
      groups.locked.push(plan);
    } else if (isQueuedPlan(plan)) {
      groups.queued.push(plan);
    } else if (number(plan.status) === 1) {
      groups.available.push(plan);
    }
  }

  const prioritySort = (a, b) =>
    priority(b) - priority(a) || number(a.id) - number(b.id);
  groups.available.sort(prioritySort);
  groups.locked.sort(prioritySort);
  groups.queued.sort(
    (a, b) =>
      number(a.queue_position) - number(b.queue_position) ||
      number(a.id) - number(b.id),
  );
  groups.inactive.sort((a, b) => {
    const aExpiry = number(a.expires_at);
    const bExpiry = number(b.expires_at);
    if (aExpiry === 0 && bExpiry !== 0) return 1;
    if (bExpiry === 0 && aExpiry !== 0) return -1;
    return bExpiry - aExpiry || number(b.id) - number(a.id);
  });
  return groups;
};
