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

import { LocaleProvider, Typography } from '@douyinfe/semi-ui';
import zh_CN from '@douyinfe/semi-ui/lib/es/locale/source/zh_CN';
import {
  CreditCard,
  LayoutDashboard,
  Menu,
  Package,
  UserRound,
} from 'lucide-react';
import React, { useEffect } from 'react';
import ReactDOM from 'react-dom/client';
import { useTranslation } from 'react-i18next';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { ToastContainer } from 'react-toastify';
import '@semi-ui-css';
import 'react-toastify/dist/ReactToastify.css';
import { StatusContext } from '../../context/Status';
import { UserContext } from '../../context/User';
import { API } from '../../helpers';
import i18n from '../../i18n/i18n';
import '../../index.css';
import MyPlans from './index';

const DAY = 86400000;
const FIXED_NOW = Date.UTC(2026, 6, 12, 23, 30, 0);
const NativeDate = Date;

function FixtureDate(...args) {
  if (!new.target) return new NativeDate(FIXED_NOW).toString();
  return Reflect.construct(
    NativeDate,
    args.length ? args : [FIXED_NOW],
    new.target,
  );
}

Object.setPrototypeOf(FixtureDate, NativeDate);
FixtureDate.prototype = Object.create(NativeDate.prototype, {
  constructor: { value: FixtureDate, configurable: true, writable: true },
  getHours: {
    value() {
      return this.getUTCHours();
    },
  },
  toLocaleDateString: {
    value(locales, options = {}) {
      return NativeDate.prototype.toLocaleDateString.call(this, locales, {
        ...options,
        timeZone: options.timeZone || 'UTC',
      });
    },
  },
  toLocaleTimeString: {
    value(locales, options = {}) {
      return NativeDate.prototype.toLocaleTimeString.call(this, locales, {
        ...options,
        timeZone: options.timeZone || 'UTC',
      });
    },
  },
  toLocaleString: {
    value(locales, options = {}) {
      return NativeDate.prototype.toLocaleString.call(this, locales, {
        ...options,
        timeZone: options.timeZone || 'UTC',
      });
    },
  },
});
FixtureDate.now = () => FIXED_NOW;
globalThis.Date = FixtureDate;

const searchParams = new URLSearchParams(window.location.search);
const modes = new Set(
  (searchParams.get('mode') || 'full')
    .split(',')
    .map((mode) => mode.trim())
    .filter(Boolean),
);
const requestedLanguage = searchParams.get('lng') || 'zh';

localStorage.setItem('quota_display_type', 'TOKENS');
localStorage.setItem('quota_per_unit', '500000');
localStorage.setItem('i18nextLng', requestedLanguage);
document.documentElement.lang = requestedLanguage;
await i18n.changeLanguage(requestedLanguage);

const makePlan = (id, fields = {}) => ({
  id,
  user_id: 1,
  status: 1,
  is_current: 0,
  auto_switch: 1,
  pinned: 0,
  can_switch: 1,
  can_toggle_auto: 1,
  allow_user_switch: 1,
  allow_user_toggle: 1,
  locked: 0,
  locked_by: '',
  locked_reason: '',
  admin_note: '',
  quota: 800,
  used_quota: 200,
  started_at: FIXED_NOW - DAY,
  expires_at: FIXED_NOW + 30 * DAY,
  queue_position: 0,
  plan_name: `fixture-${id}`,
  plan_display_name: `Fixture plan ${id}`,
  plan_type: 'subscription',
  plan_priority: 100 - id,
  plan_validity_days: 30,
  effective_daily_limit: 300,
  ...fields,
});

let plans = [
  makePlan(1, {
    is_current: 1,
    pinned: 1,
    plan_display_name: 'Pinned current plan',
    plan_priority: 200,
  }),
  makePlan(2, { plan_display_name: 'Available plan' }),
  makePlan(3, {
    plan_display_name: 'Switch disabled by administrator',
    can_switch: 0,
    admin_note: 'Switching is disabled for this fixture plan.',
  }),
  makePlan(4, {
    plan_display_name: 'Zero quota but lockable',
    quota: 0,
    used_quota: 1000,
  }),
  makePlan(5, {
    plan_display_name: 'Queued plan',
    started_at: 0,
    expires_at: 0,
    queue_position: 1,
  }),
  makePlan(6, {
    plan_display_name: 'Locked queued plan',
    started_at: 0,
    expires_at: 0,
    queue_position: 2,
    locked: 1,
    locked_by: 'admin',
    can_switch: 0,
    locked_reason: 'Administrative hold',
    admin_note: 'Queue activation requires an administrator review.',
  }),
  makePlan(7, {
    plan_display_name: 'Locked by user',
    locked: 1,
    locked_by: 'user',
    can_switch: 0,
  }),
  makePlan(8, {
    plan_display_name: 'Locked by administrator',
    locked: 1,
    locked_by: 'admin',
    can_switch: 0,
    locked_reason:
      'A deliberately long administrative reason that must stay clamped on the card while remaining complete in the details modal.',
    admin_note: 'Contact the billing team before requesting an unlock.',
  }),
  makePlan(9, {
    status: 2,
    expires_at: FIXED_NOW - DAY,
    plan_display_name: 'Expired',
  }),
  makePlan(10, { status: 3, plan_display_name: 'Disabled' }),
  makePlan(11, { status: 4, plan_display_name: 'Completed' }),
  makePlan(12, { status: 5, plan_display_name: 'Forfeited' }),
  makePlan(13, { status: 6, plan_display_name: 'Revoked' }),
  makePlan(14, {
    expires_at: FIXED_NOW,
    plan_display_name: 'Pseudo expired',
  }),
  makePlan(15, {
    started_at: 0,
    expires_at: 0,
    plan_display_name: 'Not activated',
  }),
  makePlan(16, {
    expires_at: FIXED_NOW + 3 * DAY,
    plan_display_name: 'Expires soon',
  }),
  makePlan(17, { plan_display_name: 'L'.repeat(80) }),
  makePlan(18, {
    plan_display_name: 'Available 18',
    plan_type: 'consumption',
  }),
  makePlan(19, {
    plan_display_name: 'Available 19',
    plan_type: 'trial',
  }),
  makePlan(20, {
    plan_display_name: 'Available 20',
    plan_type: 'enterprise',
  }),
  makePlan(21, { plan_display_name: 'Available 21', plan_priority: 1 }),
];

const mapCurrent = (update) => {
  plans = plans.map((plan) => (plan.id === 1 ? { ...plan, ...update } : plan));
};

if (modes.has('empty')) plans = [];
if (modes.has('no-current')) mapCurrent({ is_current: 0, pinned: 0 });
if (modes.has('locked-current')) {
  mapCurrent({
    locked: 1,
    locked_by: 'admin',
    locked_reason: 'Current plan is held by an administrator',
    admin_note: 'The current plan cannot be changed during review.',
  });
}
if (modes.has('admin-toggle-current')) {
  mapCurrent({ can_toggle_auto: 0, allow_user_toggle: 0 });
}
if (modes.has('auto-switch-off')) {
  mapCurrent({ auto_switch: 0, pinned: 0 });
}

const sectionModes = [
  ['current-only', new Set([1])],
  ['available-only', new Set([2, 3, 4, 15, 16, 17, 18, 19, 20, 21])],
  ['queued-only', new Set([5])],
  ['locked-only', new Set([6, 7, 8])],
  ['inactive-only', new Set([9, 10, 11, 12, 13, 14])],
];
const activeSectionMode = sectionModes.find(([mode]) => modes.has(mode));
if (activeSectionMode) {
  plans = plans.filter((plan) => activeSectionMode[1].has(plan.id));
}

let failNextAction = modes.has('stale-failure');
let resolveInitialRequests = null;
const initialRequestsGate = modes.has('loading')
  ? new Promise((resolve) => {
      resolveInitialRequests = resolve;
    })
  : Promise.resolve();
const requestCounters = {
  total: 0,
  gets: 0,
  actions: 0,
  byRoute: {},
};
const fixtureState = {
  fixedNow: FIXED_NOW,
  language: requestedLanguage,
  modes: [...modes],
  currentLocation: '/console/myplans',
  navigation: [],
  requestCounters,
  requests: [],
  initialReleased: !modes.has('loading'),
  releaseInitial() {
    if (this.initialReleased) return;
    this.initialReleased = true;
    resolveInitialRequests?.();
    resolveInitialRequests = null;
  },
};

window.__MYPLANS_FIXTURE__ = fixtureState;

const response = (config, data) => ({
  data,
  status: 200,
  statusText: 'OK',
  headers: {},
  config,
  request: {},
});

const recordRequest = (method, url) => {
  const route = `${method.toUpperCase()} ${url}`;
  requestCounters.total += 1;
  requestCounters.byRoute[route] = (requestCounters.byRoute[route] || 0) + 1;
  if (method === 'get') requestCounters.gets += 1;
  else requestCounters.actions += 1;
  fixtureState.requests.push({
    sequence: requestCounters.total,
    method,
    url,
  });
};

API.defaults.adapter = async (config) => {
  const method = (config.method || 'get').toLowerCase();
  const url = config.url;
  recordRequest(method, url);

  if (method === 'get' && modes.has('loading') && requestCounters.gets <= 3) {
    await initialRequestsGate;
  }

  if (method === 'get' && url === '/api/my_plans/') {
    return response(config, { success: true, data: { plans } });
  }
  if (method === 'get' && url === '/api/my_plans/quota-status') {
    return response(config, {
      success: true,
      data: {
        daily_quota_limit: 300,
        daily_quota_used: 120,
        daily_quota_remaining: 180,
        daily_reset_time: Math.floor((FIXED_NOW + DAY) / 1000),
        rate_limited: true,
        rate_limit_wait_seconds: 90,
        rate_limit_message: 'Fixture rate limit',
      },
    });
  }
  if (method === 'get' && url === '/api/my_plans/billing-status') {
    const zeroDailyPool = modes.has('zero-daily-pool');
    return response(config, {
      success: true,
      data: {
        daily_pool: modes.has('no-daily-pool')
          ? null
          : {
              total: zeroDailyPool ? 0 : 500,
              used: zeroDailyPool ? 0 : 125,
              available: zeroDailyPool ? 0 : 375,
              expires_at: '2026-07-13 00:00 UTC',
            },
        queued_plans: [
          {
            id: 5,
            queue_position: 1,
            estimated_activation_time: FIXED_NOW + 10 * DAY,
          },
          { id: 6, queue_position: 2, estimated_activation_time: 0 },
        ],
      },
    });
  }

  if (method === 'get') {
    throw new Error(
      `Unhandled fixture request: ${method.toUpperCase()} ${url}`,
    );
  }

  await new Promise((resolve) => window.setTimeout(resolve, 600));
  if (failNextAction) {
    failNextAction = false;
    const error = new Error('Plan state changed; refresh and try again.');
    error.config = config;
    error.response = {
      status: 409,
      data: {
        success: false,
        message: '套餐状态已变更，请刷新后重试',
      },
    };
    throw error;
  }

  const body =
    typeof config.data === 'string'
      ? JSON.parse(config.data)
      : config.data || {};
  if (method === 'post' && url === '/api/my_plans/switch') {
    plans = plans.map((plan) => ({
      ...plan,
      is_current: plan.id === body.user_plan_id ? 1 : 0,
      pinned: plan.id === body.user_plan_id ? 1 : 0,
    }));
    return response(config, { success: true });
  }

  const id = Number(url.match(/\/api\/my_plans\/(\d+)/)?.[1]);
  if (method === 'put' && url.endsWith('/auto_switch')) {
    plans = plans.map((plan) =>
      plan.id === id
        ? {
            ...plan,
            auto_switch: body.enabled ? 1 : 0,
            pinned: body.enabled ? 0 : plan.pinned,
          }
        : plan,
    );
    return response(config, { success: true });
  }
  if (method === 'post' && url.endsWith('/unlock')) {
    plans = plans.map((plan) =>
      plan.id === id
        ? { ...plan, locked: 0, locked_by: '', locked_reason: '' }
        : plan,
    );
    return response(config, { success: true });
  }
  if (method === 'post' && url.endsWith('/lock')) {
    plans = plans.map((plan) =>
      plan.id === id
        ? {
            ...plan,
            locked: 1,
            locked_by: 'user',
            locked_reason: 'Locked in fixture',
          }
        : plan,
    );
    return response(config, { success: true });
  }

  throw new Error(`Unhandled fixture request: ${method.toUpperCase()} ${url}`);
};

const userState = {
  user: {
    id: 1,
    quota: modes.has('zero-wallet') ? 0 : 123456,
  },
};
const statusState = {
  status: {
    recharge_disabled: modes.has('recharge-disabled'),
    HeaderNavModules: JSON.stringify({
      plans: !modes.has('route-off'),
    }),
  },
};
const noop = () => undefined;

const LocationProbe = () => {
  const location = useLocation();
  const value = `${location.pathname}${location.search}${location.hash}`;

  useEffect(() => {
    fixtureState.currentLocation = value;
    fixtureState.navigation.push(value);
    document.documentElement.dataset.fixtureLocation = value;
  }, [value]);

  return (
    <output
      id='myplans-location-probe'
      className='sr-only'
      data-location={value}
      aria-live='polite'
    >
      {value}
    </output>
  );
};

const FixtureShell = ({ children }) => {
  const { t } = useTranslation();
  const sidebarItems = [
    [LayoutDashboard, t('数据看板')],
    [CreditCard, t('钱包管理')],
    [Package, t('我的套餐')],
    [UserRound, t('个人设置')],
  ];

  return (
    <div
      data-testid='fixture-shell'
      className='flex h-screen flex-col overflow-visible bg-semi-color-bg-0 md:overflow-hidden'
    >
      <header
        data-testid='fixture-header'
        className='fixed inset-x-0 top-0 z-[100] flex h-16 items-center border-b border-semi-color-border bg-semi-color-bg-0 px-4 shadow-sm'
      >
        <Menu className='mr-3 md:hidden' size={20} aria-hidden='true' />
        <Typography.Title heading={5} className='m-0'>
          New API
        </Typography.Title>
        <Typography.Text type='tertiary' size='small' className='ml-3'>
          MyPlans fixture
        </Typography.Text>
      </header>

      <aside
        data-testid='fixture-sidebar'
        className='fixed bottom-0 left-0 top-16 z-[99] hidden w-[180px] border-r border-semi-color-border bg-semi-color-bg-0 p-3 md:block'
      >
        <nav aria-label='Fixture console navigation' className='space-y-1'>
          {sidebarItems.map(([Icon, label]) => {
            const selected = label === t('我的套餐');
            return (
              <div
                key={label}
                className={[
                  'flex min-h-10 items-center gap-3 rounded px-3 text-sm',
                  selected
                    ? 'bg-semi-color-primary-light-default font-semibold text-semi-color-primary'
                    : 'text-semi-color-text-1',
                ].join(' ')}
                aria-current={selected ? 'page' : undefined}
              >
                <Icon size={17} aria-hidden='true' />
                <span>{label}</span>
              </div>
            );
          })}
        </nav>
      </aside>

      <div
        data-testid='fixture-main-layout'
        className='flex min-h-screen flex-1 flex-col md:ml-[180px] md:min-h-0'
      >
        <div
          data-testid='fixture-scroll'
          className='flex flex-1 flex-col overflow-visible md:min-h-0 md:overflow-auto'
        >
          <main
            data-testid='fixture-content'
            className='relative flex-[1_0_auto] overflow-y-visible p-[5px] md:overflow-y-hidden md:p-6'
          >
            {children}
          </main>
          <footer
            data-testid='fixture-footer'
            className='flex-none border-t border-semi-color-border bg-semi-color-bg-0 px-6 py-16 text-center text-sm text-semi-color-text-2'
          >
            New API · MyPlans fixture
          </footer>
        </div>
      </div>
      <ToastContainer />
    </div>
  );
};

ReactDOM.createRoot(document.getElementById('root')).render(
  <LocaleProvider locale={zh_CN}>
    <StatusContext.Provider value={[statusState, noop]}>
      <UserContext.Provider value={[userState, noop]}>
        <MemoryRouter
          initialEntries={['/console/myplans']}
          future={{
            v7_startTransition: false,
            v7_relativeSplatPath: true,
          }}
        >
          <LocationProbe />
          <FixtureShell>
            <MyPlans />
          </FixtureShell>
        </MemoryRouter>
      </UserContext.Provider>
    </StatusContext.Provider>
  </LocaleProvider>,
);
