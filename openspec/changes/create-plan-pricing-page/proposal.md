# Proposal: Create Plan Pricing Page

## Problem Statement

Currently, users cannot view available subscription plans in a dedicated pricing page. The system has:
- **Admin plan management** (`/console/plan`) - for administrators to create and manage plans
- **User plan dashboard** (`/console/myplans`) - for users to view their purchased plans
- **Model pricing page** (`/pricing`) - displays AI model pricing, not subscription plans

**Gap**: There is no public-facing or user-facing pricing page that showcases available subscription plans with clear pricing, features, and comparison.

### User Pain Points

1. **Discovery Problem**: Users don't know what plans are available for purchase
2. **No Comparison**: Cannot compare plans side-by-side to make informed decisions
3. **Hidden Information**: Plan details (price, quota, features) are only visible after login or in admin panel
4. **Poor UX**: No dedicated "Pricing" section for subscription plans (separate from model pricing)

### Business Impact

- **Reduced Conversions**: Users cannot easily discover and evaluate plans
- **Support Burden**: Users ask admins "what plans do you have?"
- **Competitive Disadvantage**: Standard SaaS pricing pages are expected by users

## Proposed Solution

Create a **dedicated Plan Pricing Page** that displays all enabled subscription plans in an attractive, user-friendly layout.

### Core Features

1. **Public Pricing Page**
   - Route: `/plans` or `/subscription-plans` (configurable)
   - Optionally requires authentication (controlled by system setting)
   - Shows all enabled plans (`status=1`)

2. **Plan Display**
   - **Card-based layout** (similar to SaaS pricing pages)
   - Each plan shows:
     - Display name
     - Price (with original price if discounted)
     - Quota information (in USD or tokens)
     - Plan category badge (Daily, Monthly, Pay-as-you-go)
     - Plan type indicator (Subscription, Consumption, Trial)
     - Validity period
     - Key features list
     - CTA button ("Purchase" / "Get Started" / "Contact Sales")

3. **Plan Categories**
   - Group plans by category: Daily, Weekly, Monthly, Pay-as-you-go
   - Toggle to show/hide categories
   - Sort by priority or price

4. **Comparison Mode** (Phase 2)
   - Side-by-side comparison table
   - Highlight differences between plans

5. **Responsive Design**
   - Mobile-friendly layout
   - Adapts from 1/2/3 column layouts

### Integration Points

#### Backend
- **Existing API**: `GET /api/plan/enabled` - returns all enabled plans
- **No new APIs needed** (Phase 1)
- Optional: Add `GET /api/plan/public` if we need to expose plans without authentication

#### Frontend
- **New route**: `/plans` (or configurable in settings)
- **New page component**: `PlanPricing/index.jsx`
- **Reusable components**: Plan cards, category filters, comparison table

#### Configuration
- **System Setting**: `PricingPageRequireAuth` (true/false)
  - If true, users must log in to view plans
  - If false, anyone can view plans (public pricing page)
- **System Setting**: `PricingPageRoute` (default: `/plans`)

### User Flows

#### Flow 1: Anonymous User Views Pricing
1. User visits `/plans` (or clicks "Pricing" in navigation)
2. System checks `PricingPageRequireAuth` setting
3. If public: Display plan cards immediately
4. If auth required: Redirect to login → return to pricing page
5. User sees all enabled plans with details
6. User clicks "Purchase" → redirected to `/console/topup` with selected plan

#### Flow 2: Logged-in User Views Pricing
1. User visits `/plans` from navigation
2. System displays all enabled plans
3. System highlights plans user already owns (if any)
4. User clicks "Purchase" → add to queue or activate immediately

#### Flow 3: Admin Configures Visibility
1. Admin goes to Settings → Operation → Pricing Page
2. Admin toggles "Require Authentication" setting
3. Admin sets custom route (optional)
4. Changes take effect immediately

## Success Criteria

### Phase 1 (MVP)
- [ ] Users can view all enabled subscription plans on `/plans` route
- [ ] Each plan displays price, quota, category, and key details
- [ ] Plans are sorted by priority (or configurable sort)
- [ ] Responsive layout works on mobile/tablet/desktop
- [ ] "Purchase" button redirects to appropriate flow

### Phase 2 (Enhancements)
- [ ] Side-by-side comparison table
- [ ] Category filtering (show only Monthly, only Daily, etc.)
- [ ] FAQ section below pricing
- [ ] Testimonials/social proof (optional)
- [ ] "Most Popular" badge on recommended plan

## Alternatives Considered

### Alternative 1: Extend `/pricing` to include plans
- **Pros**: Single pricing page for everything
- **Cons**: Confusing to mix model pricing with subscription pricing
- **Decision**: Rejected - separate pages are clearer

### Alternative 2: Embed pricing in `/console/topup`
- **Pros**: No new route needed
- **Cons**: Only visible after login, not a true "pricing page"
- **Decision**: Rejected - pricing should be discoverable before login

### Alternative 3: Use external landing page (not in app)
- **Pros**: Complete design freedom
- **Cons**: Not integrated with system, manual updates required
- **Decision**: Rejected - integrated solution is better

## Dependencies

- **Existing**: Plan system fully implemented (`add-user-plan-system` complete)
- **Existing**: Queue and daily pool system complete (`add-plan-queue-and-daily-pool`)
- **Required**: Semi Design components (already used in project)
- **Optional**: Admin settings for configuring page behavior

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Plans shown to public reveal business model | Medium | Add authentication requirement setting |
| Plans change frequently, page becomes outdated | Low | Page reads from DB, always current |
| Mobile layout breaks with many plans | Low | Use responsive grid, pagination if needed |
| Users expect features not in system | Medium | Only show fields available in Plan model |

## Timeline Estimate

- **Phase 1 (MVP)**: 2-3 days
  - Backend: 0.5 day (configure route, check auth)
  - Frontend: 1.5 days (create page, design cards, responsive layout)
  - Testing: 0.5 day (manual testing, browser compatibility)
  - Documentation: 0.5 day (update docs, add i18n)

- **Phase 2 (Enhancements)**: 1-2 days
  - Comparison table, filtering, badges

## Open Questions

1. **Route Name**: Should it be `/plans`, `/pricing/plans`, or `/subscription-plans`?
   - **Recommendation**: `/plans` (simple and clear)

2. **Authentication**: Should pricing page be public by default?
   - **Recommendation**: Make it configurable, default to public

3. **Plan Purchase Flow**: Click "Purchase" → where to go?
   - **Recommendation**: Redirect to `/console/topup` with pre-selected plan ID

4. **Show Disabled Plans**: Should we show `status=2` plans with "Coming Soon"?
   - **Recommendation**: No, only show enabled plans

5. **Show Queue Info**: Should we explain queue system on pricing page?
   - **Recommendation**: Yes, add a small info section explaining queue/daily pool

## Related Changes

- `add-user-plan-system` (✓ Complete) - Foundation for plan system
- `add-plan-queue-and-daily-pool` (In Progress) - Queue logic referenced in page
- `decouple-plan-template-from-user-instance` (In Progress) - Ensures plan display is stable

---

**Proposed By**: Claude Code
**Date**: 2025-12-10
**Status**: Draft
