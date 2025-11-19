# Implementation Tasks

## 1. Fix Claude Code Settings Example (Capability: fix-claude-settings)

### 1.1 Update Tutorial Page Configuration Examples
- [x] 1.1.1 Update Windows settings.json example
  - Open `web/src/pages/Tutorial/index.jsx`
  - Locate Windows configuration code block (line ~305-312)
  - Replace `apiConfiguration` object with `env` object
  - Change `baseURL` to `ANTHROPIC_BASE_URL`
  - Change `apiKey` to `ANTHROPIC_AUTH_TOKEN`
  - Verify dynamic `${claudeApiUrl}` still interpolates correctly
- [x] 1.1.2 Update macOS/Linux settings.json example
  - Locate macOS/Linux configuration code block (line ~324-334)
  - Update bash script that creates settings.json
  - Replace `apiConfiguration` with `env` structure
  - Update key names to `ANTHROPIC_BASE_URL` and `ANTHROPIC_AUTH_TOKEN`
  - Test heredoc syntax still works correctly
- [x] 1.1.3 Verify code example consistency
  - Ensure both OS examples use identical JSON structure
  - Verify copy functionality still works
  - Check that dynamic URL generation works

### 1.2 Testing
- [x] 1.2.1 Manual testing
  - View Tutorial page in browser
  - Verify code examples display correctly
  - Copy code example and verify it's valid JSON
  - Test on Windows/macOS/Linux code paths
- [x] 1.2.2 Functional testing
  - Create actual settings.json file with copied config
  - Replace YOUR_API_KEY with real token
  - Run `claude` command and verify it connects successfully
  - Confirm no authentication errors

---

## 2. Rename Tutorial Navigation Label (Capability: rename-tutorial-nav)

### 2.1 Update Navigation Component
- [x] 2.1.1 Update useNavigation hook
  - Open `web/src/hooks/common/useNavigation.js`
  - Locate tutorial navigation item (line ~64-67)
  - Change `text: t('使用教程')` to `text: t('安装教程')`
- [x] 2.1.2 Update navigation highlighting
  - Verify `to: '/tutorial'` route match still works
  - Test active state styling

### 2.2 Update i18n Translations
- [x] 2.2.1 Update Chinese translation
  - Open `web/src/i18n/locales/zh.json`
  - Add new translation key: `"安装教程": "安装教程"`
  - Remove old key `"使用教程"` if no longer used elsewhere (check first!)
- [x] 2.2.2 Update English translation
  - Open `web/src/i18n/locales/en.json`
  - Add new translation key: `"安装教程": "Installation Tutorial"`
  - Remove old key `"使用教程"` if safe to remove

### 2.3 Testing
- [x] 2.3.1 Visual testing
  - View navigation in Chinese locale
  - Verify "安装教程" appears in nav
  - Switch to English locale
  - Verify "Installation Tutorial" appears
  - Click nav item and verify navigation to `/tutorial` works
  - Verify active state highlights correctly

---

## 3. Add Newbie Guide Button (Capability: add-newbie-guide-button)

### 3.1 Add Navigation Button
- [x] 3.1.1 Update TopNav component
  - Open `web/src/components/layout/headerbar/TopNav.jsx`
  - Add "新手指引" button to navigation items
  - Import `OnboardingWizard` component
  - Add state: `const [showOnboarding, setShowOnboarding] = useState(false)`
  - Add click handler: opens wizard if logged in, else redirects to `/login`
  - Add `OnboardingWizard` component with visibility state
- [x] 3.1.2 Add i18n translations
  - Add to `zh.json`: `"新手指引": "新手指引"`
  - Add to `en.json`: `"新手指引": "Newbie Guide"`
- [x] 3.1.3 Add analytics tracking
  - Import `OnboardingAnalytics` helper
  - Track `newbie_guide_opened` event with `source: "nav_button"`

### 3.2 Update Homepage Button Behavior
- [x] 3.2.1 Modify homepage tutorial button
  - Open `web/src/pages/Home/index.jsx`
  - Locate "使用教程" button click handler (line ~196-208)
  - Import `OnboardingWizard` component
  - Add state: `const [onboardingVisible, setOnboardingVisible] = useState(false)`
  - Update click handler logic:
    ```javascript
    onClick={() => {
      if (isLoggedIn) {
        setOnboardingVisible(true);
        OnboardingAnalytics.trackEvent('newbie_guide_opened', { source: 'homepage_button' });
      } else {
        window.location.href = '/login';
      }
    }}
    ```
  - Add `OnboardingWizard` component at bottom of render (already exists, just update state)
- [x] 3.2.2 Verify existing OnboardingWizard integration
  - Confirm `OnboardingWizard` component is already imported
  - Confirm `onboardingVisible` state and `onClose` handler exist
  - Update visibility prop binding

### 3.3 Testing
- [x] 3.3.1 Test navigation button
  - Click "新手指引" in nav as logged-in user → wizard opens
  - Click "新手指引" in nav as non-logged-in user → redirects to login
- [x] 3.3.2 Test homepage button
  - Click "使用教程" on homepage as logged-in user → wizard opens
  - Click "使用教程" on homepage as non-logged-in user → redirects to login
- [x] 3.3.3 Verify wizard functionality
  - Navigate through all wizard steps
  - Close wizard and verify state persists
  - Re-open wizard and verify progress restored
- [x] 3.3.4 Test responsive design
  - Test on mobile viewport (375px)
  - Test on tablet viewport (768px)
  - Test on desktop viewport (1920px)
- [x] 3.3.5 Verify analytics
  - Check browser console for analytics events
  - Confirm correct `source` parameter in events

---

## 4. Update Top-Up Options (Capability: update-topup-options)

### 4.1 Backend Configuration
- [x] 4.1.1 Add xianyu_shop_link configuration field
  - Locate backend settings/configuration schema
  - Add new optional string field: `xianyu_shop_link`
  - Add validation: must start with `http://` or `https://` if provided
  - Add max length: 512 characters
  - Add help text: "闲鱼店铺购买链接（可选），留空则不显示闲鱼店铺按钮"
- [x] 4.1.2 Update status API endpoint
  - Ensure `/api/status` endpoint includes `xianyu_shop_link` in response
  - Return empty string if not configured (not null)
- [x] 4.1.3 Add admin UI for configuration
  - Add input field in settings page
  - Add URL validation on frontend
  - Add save functionality

### 4.2 Update TopupStep Component
- [x] 4.2.1 Remove online payment option
  - Open `web/src/components/onboarding/steps/TopupStep.jsx`
  - Remove "在线支付" Card component (lines ~190-235)
  - Remove `handleOnlinePayment` function
  - Remove `handleTopupConfirmed` function
  - Remove `topUpLink` constant
- [x] 4.2.2 Update contact admin option to combined option
  - Rename Card title: "联系管理员或闲鱼店铺购买"
  - Update icon to `IconUserCardPhone` or add shopping icon
  - Update description: "如需帮助，请联系平台管理员或访问闲鱼店铺购买"
  - Add Space component with two buttons:
    1. "联系管理员" button (existing functionality)
    2. "闲鱼店铺" button (new)
- [x] 4.2.3 Implement Xianyu shop button logic
  - Add `xianyuShopLink` from status context:
    ```javascript
    const xianyuShopLink = statusState?.status?.xianyu_shop_link || '';
    ```
  - Conditionally render Xianyu button only if link configured:
    ```javascript
    {xianyuShopLink && (
      <Button onClick={() => window.open(xianyuShopLink, '_blank')}>
        闲鱼店铺
      </Button>
    )}
    ```
  - Add URL validation before opening

### 4.3 Update i18n Translations
- [x] 4.3.1 Add new translation keys
  - Add to `zh.json`:
    ```json
    "联系管理员或闲鱼店铺购买": "联系管理员或闲鱼店铺购买",
    "如需帮助，请联系平台管理员或访问闲鱼店铺购买": "如需帮助，请联系平台管理员或访问闲鱼店铺购买",
    "闲鱼店铺": "闲鱼店铺"
    ```
  - Add to `en.json`:
    ```json
    "联系管理员或闲鱼店铺购买": "Contact Admin or Xianyu Shop",
    "如需帮助，请联系平台管理员或访问闲鱼店铺购买": "For assistance, please contact admin or visit Xianyu shop",
    "闲鱼店铺": "Xianyu Shop"
    ```

### 4.4 Testing
- [x] 4.4.1 Test with Xianyu link configured
  - Set `xianyu_shop_link` in backend
  - Open onboarding wizard step 2
  - Verify "闲鱼店铺" button is visible
  - Click button → opens link in new tab
- [x] 4.4.2 Test without Xianyu link configured
  - Clear `xianyu_shop_link` in backend
  - Open onboarding wizard step 2
  - Verify "闲鱼店铺" button is hidden
  - Only "联系管理员" button visible
- [x] 4.4.3 Test redemption code flow (unchanged)
  - Enter valid redemption code → success, auto-advance
  - Enter invalid code → error message, stay on step
  - Verify quota update in user context
- [x] 4.4.4 Test step navigation
  - "上一步" button → back to step 1
  - "跳过此步" button → advance to step 3
  - Redemption success → auto-advance to step 3

---

## 5. Integration Testing

### 5.1 End-to-End Testing
- [x] 5.1.1 Test complete onboarding flow with all changes
  - New user registers and logs in
  - Onboarding wizard auto-opens (existing functionality)
  - Navigate through all steps
  - Verify all changes integrated correctly
- [x] 5.1.2 Test manual wizard access
  - Click "新手指引" in navigation → wizard opens
  - Click "使用教程" on homepage → wizard opens
  - Complete wizard → close and verify completion state
- [x] 5.1.3 Test Tutorial page
  - Navigate to "安装教程" (renamed nav item)
  - View Claude Code settings examples
  - Copy settings and verify format is correct
  - Test on all three OS tabs

### 5.2 Regression Testing
- [x] 5.2.1 Verify no breaking changes
  - Existing onboarding auto-trigger still works
  - Token creation flow unchanged
  - Navigation routing still works
  - Analytics tracking still works
- [x] 5.2.2 Test on different browsers
  - Chrome (latest)
  - Firefox (latest)
  - Safari (latest)
  - Edge (latest)
- [x] 5.2.3 Test responsive layouts
  - Mobile (375px, 414px)
  - Tablet (768px, 1024px)
  - Desktop (1280px, 1920px)

---

## 6. Documentation and Deployment

### 6.1 Update Documentation
- [x] 6.1.1 Update admin documentation
  - Document new `xianyu_shop_link` configuration field
  - Explain when to use it (when no online payment)
  - Provide example URL format
- [x] 6.1.2 Update user-facing help
  - Update FAQ if needed
  - Update in-app help text if needed

### 6.2 Deployment
- [x] 6.2.1 Backend deployment
  - Deploy backend changes (xianyu_shop_link configuration)
  - Run database migrations if needed
  - Verify `/api/status` returns new field
- [x] 6.2.2 Frontend deployment
  - Build frontend with all changes
  - Deploy to staging environment first
  - Test all changes in staging
  - Deploy to production
- [x] 6.2.3 Post-deployment verification
  - Verify Tutorial page shows correct Claude Code settings
  - Verify navigation shows "安装教程"
  - Verify "新手指引" buttons work
  - Verify TopupStep shows updated options
  - Monitor analytics events
  - Monitor error logs for issues

---

## Validation Criteria

**All tasks must meet these criteria before marking as complete:**

- [x] Code follows project style guide (React, Tailwind, Semi Design)
- [x] No console errors or warnings in browser
- [x] All user-facing text supports i18n (wrapped in `t()`)
- [x] Accessibility: keyboard navigation works, ARIA labels present
- [x] Responsive: works on mobile (375px), tablet (768px), desktop (1920px)
- [x] Analytics events fire correctly with proper properties
- [x] No breaking changes to existing flows
- [x] Tutorial page Claude Code settings are CORRECT and functional
- [x] Navigation labels accurately describe content
- [x] Onboarding wizard accessible from multiple entry points
- [x] Payment options reflect current system capabilities
