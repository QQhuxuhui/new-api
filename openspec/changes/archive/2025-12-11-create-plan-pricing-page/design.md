# Design: Plan Pricing Page

## Architecture Overview

This change introduces a **user-facing pricing page** that displays subscription plans in an attractive, conversion-optimized layout. The design follows existing project patterns and reuses components where possible.

```
┌─────────────────────────────────────────────────────────────┐
│                     User Browser                             │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  Navigation Bar                                         │ │
│  │  [Home] [Documentation] [Pricing] [Login]              │ │
│  └────────────────────────────────────────────────────────┘ │
│                          │                                   │
│                          ▼                                   │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  /plans Route                                          │ │
│  │                                                         │ │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐                │ │
│  │  │  Daily  │  │ Monthly │  │  PayG   │                │ │
│  │  │ Plan    │  │  Plan   │  │  Plan   │                │ │
│  │  │ Card    │  │  Card   │  │  Card   │                │ │
│  │  └─────────┘  └─────────┘  └─────────┘                │ │
│  │                                                         │ │
│  │  [Purchase Button] → /console/topup?plan_id=X         │ │
│  └────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
            ┌─────────────────────────────┐
            │  GET /api/plan/enabled      │
            │                             │
            │  Returns: Plan[]            │
            │  - id, display_name         │
            │  - price, original_price    │
            │  - category, type           │
            │  - default_quota            │
            │  - validity_days            │
            │  - description              │
            └─────────────────────────────┘
                          │
                          ▼
            ┌─────────────────────────────┐
            │  model.GetAllEnabledPlans() │
            │                             │
            │  Query:                     │
            │  SELECT * FROM plans        │
            │  WHERE status = 1           │
            │  ORDER BY priority DESC     │
            └─────────────────────────────┘
```

## Component Architecture

### Frontend Structure

```
web/src/
├── pages/
│   └── PlanPricing/
│       ├── index.jsx                    # Main page component
│       ├── PlanPricingPage.jsx          # Layout container
│       └── components/
│           ├── PlanCard.jsx             # Individual plan card
│           ├── PlanCardSkeleton.jsx     # Loading skeleton
│           ├── PlanComparison.jsx       # Comparison table (Phase 2)
│           ├── CategoryFilter.jsx       # Filter by category
│           ├── PricingHeader.jsx        # Page header/title
│           └── PricingFAQ.jsx           # FAQ section (Phase 2)
├── hooks/
│   └── plans/
│       └── usePlanPricingData.jsx       # Data fetching hook
└── components/
    └── plan/
        ├── PlanFeatureList.jsx          # Feature list component
        ├── PlanPriceBadge.jsx           # Price display with discount
        ├── PlanCategoryBadge.jsx        # Category indicator
        └── PurchaseButton.jsx           # CTA button
```

### Backend (No Changes Required)

Existing API is sufficient:

```go
// controller/plan.go:306-318
func GetEnabledPlans(c *gin.Context) {
    plans, err := model.GetAllEnabledPlans()
    // Returns: {success: true, data: Plan[]}
}
```

## Data Flow

### 1. Page Load Sequence

```
User navigates to /plans
    │
    ├─> Check authentication (if required)
    │   ├─> Not logged in → Redirect to /login?redirect=/plans
    │   └─> Logged in → Continue
    │
    ├─> Fetch plans via GET /api/plan/enabled
    │   └─> Returns Plan[] sorted by priority DESC
    │
    ├─> Render plan cards
    │   ├─> Group by category (optional)
    │   ├─> Sort by priority/price
    │   └─> Highlight recommended plan
    │
    └─> User clicks "Purchase"
        └─> Navigate to /console/topup?plan_id=X
```

### 2. Plan Data Transformation

```javascript
// Raw API Response
{
  "success": true,
  "data": [
    {
      "id": 1,
      "name": "monthly_basic",
      "display_name": "Basic Monthly",
      "description": "Perfect for individual developers",
      "type": "subscription",
      "category": "monthly",
      "priority": 100,
      "price": 9.99,
      "original_price": 19.99,
      "quota_usd": 10.00,
      "default_quota": 1000000,
      "validity_days": 30,
      "status": 1,
      // ... other fields
    }
  ]
}

// Transformed for Display
{
  id: 1,
  title: "Basic Monthly",
  subtitle: "Perfect for individual developers",
  price: {
    current: "$9.99",
    original: "$19.99",
    discount: "50% OFF"
  },
  quota: "10 USD Credits",
  features: [
    "30 days validity",
    "1M tokens quota",
    "Standard support",
    "Auto-renewal available"
  ],
  badge: {
    type: "monthly",
    label: "Monthly Plan"
  },
  cta: {
    text: "Get Started",
    href: "/console/topup?plan_id=1"
  }
}
```

## UI/UX Design

### Layout Options

#### Option 1: Card Grid (Recommended)

```
┌──────────────────────────────────────────────────────────┐
│              Choose Your Perfect Plan                     │
│                                                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐      │
│  │   DAILY     │  │   MONTHLY   │  │   PAY-G     │      │
│  │   CARD      │  │   CARD      │  │   CARD      │      │
│  │             │  │             │  │             │      │
│  │   $0.99     │  │   $9.99     │  │  Pay as     │      │
│  │   /day      │  │   /month    │  │  you go     │      │
│  │             │  │             │  │             │      │
│  │ ✓ Feature 1 │  │ ✓ Feature 1 │  │ ✓ Feature 1 │      │
│  │ ✓ Feature 2 │  │ ✓ Feature 2 │  │ ✓ Feature 2 │      │
│  │ ✓ Feature 3 │  │ ✓ Feature 3 │  │ ✓ Feature 3 │      │
│  │             │  │             │  │             │      │
│  │ [Purchase]  │  │ [Purchase]  │  │ [Purchase]  │      │
│  └─────────────┘  └─────────────┘  └─────────────┘      │
└──────────────────────────────────────────────────────────┘
```

#### Option 2: Comparison Table (Phase 2)

```
┌───────────────────────────────────────────────────────────┐
│               Plan Feature Comparison                      │
│                                                            │
│  Feature         │ Daily  │ Monthly │ Annual  │ PayG     │
│  ───────────────────────────────────────────────────────  │
│  Price           │ $0.99  │ $9.99   │ $99.99  │ Variable │
│  Quota           │ 1 USD  │ 10 USD  │ 120 USD │ Unlimited│
│  Validity        │ 1 day  │ 30 days │ 365 days│ Forever  │
│  Priority        │ High   │ Medium  │ Low     │ Lowest   │
│  Auto-renewal    │ ✗      │ ✓       │ ✓       │ N/A      │
│  Queue Slot      │ No     │ Yes     │ Yes     │ No       │
│                  │[Select]│[Select] │[Select] │[Select]  │
└───────────────────────────────────────────────────────────┘
```

### Plan Card Design

```jsx
<Card
  className="plan-card"
  hoverable
  style={{ maxWidth: 320 }}
>
  {/* Header */}
  <div className="plan-header">
    <Badge count={plan.category} color="blue" />
    {plan.isRecommended && <Tag color="orange">Most Popular</Tag>}
    <Typography.Title level={3}>
      {plan.display_name}
    </Typography.Title>
    <Typography.Text type="secondary">
      {plan.description}
    </Typography.Text>
  </div>

  {/* Pricing */}
  <div className="plan-pricing">
    {plan.original_price > plan.price && (
      <Typography.Text delete type="secondary">
        ${plan.original_price}
      </Typography.Text>
    )}
    <Typography.Title level={1}>
      ${plan.price}
    </Typography.Title>
    <Typography.Text type="secondary">
      {getPriceUnit(plan.category)}
    </Typography.Text>
  </div>

  {/* Features */}
  <List
    dataSource={extractFeatures(plan)}
    renderItem={feature => (
      <List.Item>
        <IconCheckCircle /> {feature}
      </List.Item>
    )}
  />

  {/* CTA */}
  <Button
    type="primary"
    size="large"
    block
    onClick={() => handlePurchase(plan.id)}
  >
    Get Started
  </Button>
</Card>
```

## Feature Extraction Logic

The system will derive features from Plan model fields:

```javascript
function extractFeatures(plan) {
  const features = [];

  // Quota
  if (plan.quota_usd > 0) {
    features.push(`${plan.quota_usd} USD in credits`);
  } else if (plan.default_quota > 0) {
    features.push(`${formatQuota(plan.default_quota)} tokens`);
  }

  // Validity
  if (plan.validity_days > 0) {
    features.push(`Valid for ${plan.validity_days} days`);
  } else {
    features.push("Never expires");
  }

  // Daily limit
  if (plan.daily_quota_limit > 0) {
    features.push(`Daily limit: ${formatQuota(plan.daily_quota_limit)}`);
  } else {
    features.push("No daily limits");
  }

  // Category-specific
  if (plan.category === 'daily') {
    features.push("Stacks with other plans");
    features.push("No queue slot required");
  } else if (plan.category === 'monthly') {
    features.push("Auto-renewal available");
    features.push("Occupies 1 queue slot");
  } else if (plan.category === 'payg') {
    features.push("Pay only for what you use");
    features.push("No expiration");
  }

  // Rate limits
  if (plan.rate_limit_rules && plan.rate_limit_rules !== '') {
    features.push("Rate limits apply");
  }

  // Channel access
  if (plan.channel_groups) {
    const groups = JSON.parse(plan.channel_groups);
    if (groups.length > 0) {
      features.push(`Access to ${groups.length} model groups`);
    }
  }

  return features;
}
```

## Routing & Configuration

### Route Registration

```javascript
// App.jsx
import PlanPricing from './pages/PlanPricing';

// Add route
<Route
  path="/plans"
  element={
    <RequireAuth required={siteInfo.pricingRequireAuth}>
      <PlanPricing />
    </RequireAuth>
  }
/>
```

### System Settings

Add to `option` table (or use environment variables):

```json
{
  "PricingPageEnabled": true,
  "PricingPageRequireAuth": false,
  "PricingPageRoute": "/plans",
  "PricingPageRecommendedPlanId": 2,
  "PricingPageShowComparison": false
}
```

## State Management

### Hook: usePlanPricingData

```javascript
// hooks/plans/usePlanPricingData.jsx
export function usePlanPricingData() {
  const [plans, setPlans] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [filter, setFilter] = useState('all'); // 'all', 'daily', 'monthly', 'payg'
  const [sortBy, setSortBy] = useState('priority'); // 'priority', 'price', 'quota'

  useEffect(() => {
    async function loadPlans() {
      try {
        const res = await API.get('/api/plan/enabled');
        if (res.data.success) {
          setPlans(res.data.data);
        }
      } catch (err) {
        setError(err.message);
      } finally {
        setLoading(false);
      }
    }
    loadPlans();
  }, []);

  const filteredPlans = useMemo(() => {
    let filtered = plans;

    // Filter by category
    if (filter !== 'all') {
      filtered = filtered.filter(p => p.category === filter);
    }

    // Sort
    filtered.sort((a, b) => {
      switch (sortBy) {
        case 'priority':
          return b.priority - a.priority;
        case 'price':
          return a.price - b.price;
        case 'quota':
          return b.quota_usd - a.quota_usd;
        default:
          return 0;
      }
    });

    return filtered;
  }, [plans, filter, sortBy]);

  return {
    plans: filteredPlans,
    loading,
    error,
    filter,
    setFilter,
    sortBy,
    setSortBy
  };
}
```

## Responsive Design

### Breakpoints

```css
/* Mobile: 1 column */
@media (max-width: 768px) {
  .plan-grid {
    grid-template-columns: 1fr;
  }
}

/* Tablet: 2 columns */
@media (min-width: 769px) and (max-width: 1024px) {
  .plan-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

/* Desktop: 3 columns */
@media (min-width: 1025px) {
  .plan-grid {
    grid-template-columns: repeat(3, 1fr);
  }
}

/* Wide: 4 columns (optional) */
@media (min-width: 1400px) {
  .plan-grid {
    grid-template-columns: repeat(4, 1fr);
  }
}
```

## Internationalization

### i18n Keys

```json
// en.json
{
  "pricing": {
    "title": "Choose Your Perfect Plan",
    "subtitle": "Flexible pricing for every need",
    "category": {
      "daily": "Daily Card",
      "weekly": "Weekly Plan",
      "monthly": "Monthly Plan",
      "payg": "Pay-as-you-go"
    },
    "features": {
      "quota": "{amount} in credits",
      "validity": "Valid for {days} days",
      "noExpiry": "Never expires",
      "dailyLimit": "Daily limit: {amount}",
      "noLimit": "No limits",
      "stack": "Stacks with other plans",
      "queueSlot": "Occupies 1 queue slot",
      "noQueue": "No queue slot required"
    },
    "cta": {
      "purchase": "Purchase Plan",
      "getStarted": "Get Started",
      "contactSales": "Contact Sales",
      "comingSoon": "Coming Soon"
    },
    "discount": "{percent}% OFF",
    "recommended": "Most Popular"
  }
}
```

## Performance Considerations

### Optimization Strategies

1. **Data Caching**
   - Cache `GetAllEnabledPlans()` result in Redis for 5 minutes
   - Invalidate on plan update/creation/deletion

2. **Lazy Loading**
   - Load plan details on card hover (if detailed info needed)
   - Lazy load comparison table component

3. **Image Optimization**
   - Use plan icons/images with lazy loading
   - Serve from CDN if available

4. **Bundle Size**
   - Use code splitting for PlanPricing route
   - Minimize CSS for plan cards

## Security Considerations

### Authorization

```javascript
// If authentication required
if (siteInfo.pricingRequireAuth && !isAuthenticated) {
  navigate(`/login?redirect=/plans`);
}

// Check if plan is actually enabled
if (plan.status !== 1) {
  throw new Error('Plan not available');
}
```

### Input Validation

```javascript
// When redirecting to purchase
function handlePurchase(planId) {
  // Validate plan ID
  const plan = plans.find(p => p.id === planId);
  if (!plan) {
    showError('Invalid plan selected');
    return;
  }

  // Redirect to topup with plan_id
  navigate(`/console/topup?plan_id=${planId}`);
}
```

## Testing Strategy

### Unit Tests (Phase 2)

```javascript
describe('PlanCard', () => {
  it('displays plan name and price correctly', () => {});
  it('shows discount badge when original_price > price', () => {});
  it('formats quota correctly', () => {});
  it('renders feature list', () => {});
});

describe('usePlanPricingData', () => {
  it('fetches plans on mount', () => {});
  it('filters plans by category', () => {});
  it('sorts plans by priority', () => {});
  it('handles API errors gracefully', () => {});
});
```

### Manual Testing Checklist

- [ ] Page loads without errors
- [ ] Plans display correctly (all enabled plans visible)
- [ ] Prices display correctly (with/without discount)
- [ ] Features list is accurate
- [ ] Purchase button redirects correctly
- [ ] Responsive layout works on mobile/tablet/desktop
- [ ] Loading state displays skeleton cards
- [ ] Error state displays friendly message
- [ ] Category filter works (if implemented)
- [ ] Sorting works (if implemented)

## Migration Path

### Existing Users

- No database changes required (uses existing `plans` table)
- Route `/plans` is new, no conflicts expected
- Can run alongside existing pages without impact

### Rollback Strategy

If issues occur:
1. Remove `/plans` route from App.jsx
2. Remove PlanPricing components (won't affect other pages)
3. No database rollback needed (no schema changes)

## Future Enhancements

### Phase 3+

1. **Interactive Pricing Calculator**
   - User inputs usage → system recommends plan

2. **Plan Add-ons**
   - Display additional features that can be purchased

3. **Comparison with Competitors**
   - "Why choose us" section

4. **Video Demos**
   - Embedded videos showing plan features

5. **Live Chat Integration**
   - "Have questions? Chat with us" widget

6. **A/B Testing**
   - Test different pricing layouts
   - Optimize for conversions

---

**Design Status**: Complete
**Review Required**: Yes (UX/UI team)
**Breaking Changes**: None
