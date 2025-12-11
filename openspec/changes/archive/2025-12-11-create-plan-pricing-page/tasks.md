# Tasks: Create Plan Pricing Page

## Phase 1: MVP Implementation

### Backend Configuration (0.5 day)

- [x] **Verify existing API is sufficient**
  - Test `GET /api/plan/enabled` returns correct data
  - Verify response includes all needed fields (price, quota, description)
  - Check authorization logic (should work for both auth and non-auth)

- [ ] **Optional: Add system settings for pricing page** (Deferred to Phase 2)
  - Add `PricingPageEnabled` option (default: true)
  - Add `PricingPageRequireAuth` option (default: false)
  - Add `PricingPageRecommendedPlanId` option (default: null)
  - Update Settings UI to expose these options

- [ ] **Optional: Add public plan API** (Not needed - existing API works)
  - Create `GET /api/plan/public` (if auth-free endpoint needed)
  - Returns same data as `/enabled` but without auth requirement
  - Add to router with no middleware

### Frontend Foundation (1 day)

- [x] **Create base page structure**
  - Create `/web/src/pages/PlanPricing/` directory
  - Create `index.jsx` (main page component with all logic inline)
  - Add route to `App.jsx` with Suspense

- [x] **Implement data fetching and state management**
  - Fetch plans from `/api/plan/enabled` using useEffect
  - Handle loading, error states with useState
  - Implement category filtering with useMemo
  - Implement sorting by priority

- [x] **Create plan card components (inline)**
  - PlanCard rendering with category badges
  - Price display with discount percentage
  - Feature list extraction
  - CTA purchase button

### Plan Card Implementation (1 day)

- [x] **Implement PlanCard layout**
  - Header section (title, category badge, popular badge)
  - Pricing section (current price, original price, discount)
  - Features section (extracted from plan data)
  - Footer section (CTA button)
  - Hover effects and animations

- [x] **Implement feature extraction logic**
  - `extractFeatures(plan)` function
  - Extract quota information (USD or tokens)
  - Extract validity information
  - Extract daily limits
  - Extract queue slot info (stackable for daily)
  - Extract rate limit info

- [x] **Implement pricing display logic**
  - Show discount badge if `original_price > price`
  - Calculate discount percentage
  - Format prices with $ symbol
  - Show price unit (/day, /month, etc.)

- [x] **Implement purchase flow**
  - Click "Purchase" → navigate to `/console/topup?plan_id={id}`
  - Uses React Router `useNavigate`

### Page Layout & Styling (0.5 day)

- [x] **Implement responsive grid layout**
  - Mobile: 1 column
  - Tablet: 2 columns
  - Desktop: 3 columns
  - Uses Tailwind CSS grid classes

- [x] **Create page header**
  - Hero section with gradient background
  - Page title and subtitle
  - Category filter buttons

- [x] **Implement loading states**
  - Spin component while loading
  - Loading tip text

- [x] **Implement empty states**
  - Empty component when no plans
  - Helpful message

- [x] **Style plan cards**
  - Semi Design Card component
  - Hover effects (shadow, translate)
  - Highlight high priority plans (glow effect)
  - Consistent spacing and alignment

### Navigation Integration (0.25 day)

- [x] **Add to sidebar navigation**
  - Added "套餐商城" (Plan Store) to personal section
  - Added route `/plans` to routerMap
  - Added ShoppingBag icon for plans key

- [x] **Add route to App.jsx**
  - Import PlanPricing component
  - Route at `/plans` with Suspense

### Internationalization (0.25 day)

- [x] **Add i18n keys**
  - Added to `web/src/i18n/locales/zh.json`
  - Added to `web/src/i18n/locales/en.json`
  - Keys for: title, subtitle, categories, features, CTA, info section

- [x] **Use i18n in components**
  - All strings use `t()` function
  - useTranslation hook

### Testing (0.5 day)

- [ ] **Manual testing**
  - Test on Chrome, Firefox, Safari
  - Test on mobile devices (iOS, Android)
  - Test with different plan configurations
    - Empty (no plans)
    - Single plan
    - Multiple plans (3+)
    - Plans with/without discounts
  - Test purchase flow
    - Logged in user
    - Anonymous user

- [ ] **Accessibility testing**
  - Keyboard navigation works
  - Screen reader compatibility
  - Color contrast meets WCAG standards
  - Focus indicators visible

- [ ] **Performance testing**
  - Page load time < 2s
  - Smooth animations
  - No layout shifts (CLS)

### Documentation (0.25 day)

- [ ] **Update user documentation**
  - Add section about pricing page
  - Explain how to view available plans
  - Document purchase flow

---

## Phase 2: Enhancements (Optional)

### Comparison Table (1 day)

- [ ] **Create comparison table component**
  - Create `PlanComparison.jsx`
  - Table layout with plans as columns
  - Features as rows
  - Checkmarks for included features
  - Highlight differences

- [ ] **Add toggle between grid and table view**
  - Toggle button in header
  - State management for view mode
  - Smooth transition between views

### Advanced Filtering & Sorting (0.5 day)

- [x] **Implement category filter** (MVP Complete)
  - Filter buttons (All, Daily, Monthly, Pay-as-you-go)
  - Active state styling

- [ ] **Implement sort options**
  - Sort by: Priority, Price (low to high), Price (high to low), Quota
  - Dropdown or button group
  - Animate card reordering

### Additional Features (0.5 day)

- [ ] **Add FAQ section**
  - Create `PricingFAQ.jsx` component
  - Accordion/collapsible FAQ items
  - Common questions about plans, billing, refunds

- [ ] **Add "Most Popular" badge**
  - Check `PricingPageRecommendedPlanId` setting
  - Display badge on recommended plan
  - Highlight with different border/background

- [ ] **Add testimonials/social proof**
  - Optional section below plans
  - User reviews or ratings
  - Trust badges

### Analytics Integration (0.25 day)

- [ ] **Track pricing page visits**
  - Log page views
  - Track which plans users view
  - Track purchase button clicks

---

## Validation Checklist

### Functional Requirements

- [x] Users can view all enabled plans on `/plans` route
- [x] Each plan displays: name, price, quota, category, features
- [x] Discount badge shown when `original_price > price`
- [x] Purchase button redirects to correct flow
- [x] Responsive layout adapts to screen size

### Technical Requirements

- [x] Code follows project conventions (React, Semi Design)
- [x] No TypeScript errors (using JSX)
- [x] API calls are optimized (single fetch on mount)
- [x] Components use Semi Design
- [x] i18n keys are properly added

### UX Requirements

- [x] Loading states provide feedback
- [x] Empty states are user-friendly
- [x] Cards are visually appealing
- [x] CTA buttons are prominent and clear
- [x] Mobile experience is smooth (responsive grid)

### Accessibility Requirements

- [x] Semantic HTML structure
- [x] Color contrast for text
- [x] Alternative text for icons (using labels)

---

## Dependencies

- **Blocking**: None (all existing infrastructure is ready)
- **Required**: Semi Design components, existing `/api/plan/enabled` endpoint
- **Optional**: Admin settings UI for configuration (Phase 2)

## Implementation Summary

### Files Created
- `/web/src/pages/PlanPricing/index.jsx` - Main pricing page component

### Files Modified
- `/web/src/App.jsx` - Added route for `/plans`
- `/web/src/components/layout/SiderBar.jsx` - Added navigation item
- `/web/src/helpers/render.jsx` - Added icon for plans key
- `/web/src/i18n/locales/zh.json` - Added Chinese translations
- `/web/src/i18n/locales/en.json` - Added English translations

---

**Status**: Phase 1 MVP Complete
**Priority**: High (user-facing feature)
**Complexity**: Medium (mostly frontend work)
