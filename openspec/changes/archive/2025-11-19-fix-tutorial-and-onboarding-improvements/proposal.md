# Proposal: Fix Tutorial and Onboarding Improvements

## Why

Multiple user-facing issues need to be addressed to improve the new user experience:

1. **Claude Code Configuration Error**: The Tutorial page (`/tutorial`) displays incorrect environment variable names in the settings.json example for Claude Code. The current example uses incorrect keys (`apiConfiguration.baseURL` and `apiConfiguration.apiKey`), but Claude Code actually expects `env.ANTHROPIC_BASE_URL` and `env.ANTHROPIC_AUTH_TOKEN`.

2. **Navigation Clarity**: The top navigation shows "使用教程" (Usage Tutorial) which creates confusion. Users expect step-by-step usage instructions, but the page actually contains installation and configuration tutorials. Renaming to "安装教程" (Installation Tutorial) better matches the content.

3. **Missing Entry Point for Onboarding**: Currently, the onboarding wizard (`OnboardingWizard` component) is only triggered automatically on first login. There's no manual button to access it, making it hard for users to re-run the guided setup or for logged-in users to access it from the homepage.

4. **Outdated Payment Information**: The TopupStep in the onboarding wizard shows "在线支付" (Online Payment) option, but the system no longer supports online payment. Users should instead be directed to "联系管理员" or "闲鱼店铺购买" (Xianyu shop purchase).

## What Changes

**1. Fix Claude Code Settings Example** (specs/fix-claude-settings)
- Update Tutorial page (`/tutorial`) settings.json code examples
- Change from incorrect keys to correct keys:
  ```json
  // BEFORE (WRONG):
  {
    "apiConfiguration": {
      "baseURL": "https://sparkcode.top",
      "apiKey": "YOUR_API_KEY"
    }
  }

  // AFTER (CORRECT):
  {
    "env": {
      "ANTHROPIC_BASE_URL": "https://sparkcode.top",
      "ANTHROPIC_AUTH_TOKEN": "YOUR_API_KEY"
    }
  }
  ```

**2. Rename Tutorial Navigation** (specs/rename-tutorial-nav)
- Update navigation label from "使用教程" to "安装教程"
- Update i18n translation keys
- Update page metadata and titles

**3. Add Newbie Guide Button** (specs/add-newbie-guide-button)
- Add "新手指引" (Newbie Guide) button to top navigation
- Button opens existing `OnboardingWizard` component
- Available to all users (not just first-time users)
- Homepage "使用教程" button behavior:
  - If user is logged in: Open `OnboardingWizard`
  - If user is not logged in: Navigate to `/login`

**4. Update Top-Up Options** (specs/update-topup-options)
- Remove "在线支付" (Online Payment) option from `TopupStep`
- Add new backend configuration: `xianyu_shop_link` (separate from `top_up_link`)
- Update TopupStep UI to show two options:
  1. 兑换码充值 (Redemption Code) - existing functionality
  2. 联系管理员或闲鱼店铺购买 - combined option card
     - Shows admin contact info + link to Xianyu shop (if configured)
     - Uses new `xianyu_shop_link` configuration

## Impact

**Affected Components:**
- `web/src/pages/Tutorial/index.jsx` - Fix Claude Code settings example
- `web/src/hooks/common/useNavigation.js` - Rename tutorial label
- `web/src/i18n/locales/zh.json` - Update translation keys
- `web/src/i18n/locales/en.json` - Update translation keys
- `web/src/pages/Home/index.jsx` - Update "使用教程" button behavior, add "新手指引" button
- `web/src/components/layout/headerbar/TopNav.jsx` - Add "新手指引" navigation button
- `web/src/components/onboarding/steps/TopupStep.jsx` - Update payment options UI
- Backend configuration schema - Add `xianyu_shop_link` field

**User Benefits:**
- **Correct Configuration**: Users can successfully configure Claude Code (critical bug fix)
- **Clear Navigation**: "安装教程" better describes the content than "使用教程"
- **Easy Access to Guide**: Users can re-run onboarding wizard anytime via "新手指引" button
- **Accurate Payment Info**: Users see only available payment methods (no misleading "online payment" option)

**Breaking Changes:**
- None. These are UI improvements and configuration fixes.

**Migration Required:**
- None. New configuration field `xianyu_shop_link` is optional.

**Performance Impact:**
- Negligible. Changes are primarily text updates and conditional rendering.

**Security Considerations:**
- None. No security-sensitive code affected.

**Dependencies:**
- Requires existing `OnboardingWizard` component (already implemented)
- No external dependency changes

## Alternative Approaches Considered

**Alternative 1: Keep "使用教程" and Add Separate "安装教程"**
- Pros: More granular navigation
- Cons: Creates navigation clutter, Tutorial page is already installation-focused
- Rejected: The current Tutorial page IS an installation guide, so renaming is more accurate

**Alternative 2: Create Separate Payment Page Instead of Xianyu Link**
- Pros: More control over payment flow
- Cons: More complex, requires backend changes, admin has to create custom page
- Rejected: Simple link is sufficient and gives admin flexibility

**Alternative 3: Remove TopupStep Entirely from Onboarding**
- Pros: Simplifies wizard, removes payment friction
- Cons: Users might not realize they need to top up
- Rejected: Top-up guidance is important for new users, just needs accurate options

## Success Metrics

**Primary Metrics (30 days post-launch):**
- Claude Code configuration success rate: >95% (up from current failure rate)
- Onboarding wizard manual re-open rate: >15% (new metric, shows utility)
- Top-up step confusion/error reports: -100% (no more "online payment not working" tickets)

**Secondary Metrics:**
- Tutorial page bounce rate: <30% (users find correct info quickly)
- Support tickets for "how to configure Claude Code": -90%

**Tracking:**
- Analytics events:
  - `tutorial_page_viewed` (track if users find settings)
  - `newbie_guide_opened` (source: nav_button | homepage_button)
  - `topup_option_selected` (option: redemption_code | contact_admin | xianyu)

## Timeline

- **Week 1 Day 1-2**: Frontend fixes (Tutorial page, navigation labels, translations)
- **Week 1 Day 3-4**: Add newbie guide buttons (navigation + homepage)
- **Week 1 Day 5**: Update TopupStep UI and logic
- **Week 2**: Backend configuration field for `xianyu_shop_link`, testing, deployment

## Rollout Plan

**Phase 1: Tutorial Fix (Immediate)**
- Deploy Tutorial page settings.json fix immediately (critical bug)
- No rollback risk, pure fix

**Phase 2: Navigation and UI Updates (Week 1)**
- Deploy navigation rename and newbie guide buttons
- Monitor user feedback and analytics

**Phase 3: Top-Up Options Update (Week 2)**
- Add backend configuration for Xianyu link
- Deploy TopupStep UI changes
- Notify admins about new configuration option
