# Spec Delta: Homepage FAQ Display

## ADDED Requirements

### Requirement: Homepage SHALL display admin-managed FAQ content

The homepage FAQ section MUST consume FAQ data from the backend admin system (`console_setting.faq`) instead of using hardcoded content. This ensures administrators can manage FAQ content without code changes.

**Rationale**: Centralizes FAQ content management and eliminates the need for code deployments when updating FAQ information.

**Priority**: High

#### Scenario: Homepage displays FAQ items from backend when enabled

**Given** the system has FAQ data configured in `console_setting.faq`
**And** `console_setting.faq_enabled` is `true`
**When** a user visits the homepage
**Then** the FAQ section should display up to 4 FAQ items from the backend data
**And** each FAQ item should show the question as a heading and answer as body text

#### Scenario: Homepage hides FAQ section when disabled

**Given** `console_setting.faq_enabled` is `false`
**When** a user visits the homepage
**Then** the FAQ section should not be rendered at all

#### Scenario: Homepage hides FAQ section when no data exists

**Given** `console_setting.faq` is empty or contains no items
**And** `console_setting.faq_enabled` is `true`
**When** a user visits the homepage
**Then** the FAQ section should not be rendered

#### Scenario: Homepage displays fewer than 4 items when available data is limited

**Given** `console_setting.faq` contains 2 FAQ items
**And** `console_setting.faq_enabled` is `true`
**When** a user visits the homepage
**Then** the FAQ section should display all 2 available items
**And** should not show empty placeholders

#### Scenario: Homepage displays only first 4 items when more are available

**Given** `console_setting.faq` contains 10 FAQ items
**And** `console_setting.faq_enabled` is `true`
**When** a user visits the homepage
**Then** the FAQ section should display only the first 4 items
**And** should not paginate or show a "load more" button

## REMOVED Requirements

### Requirement: Homepage MUST NOT use hardcoded FAQ content

The hardcoded FAQ array previously embedded in the homepage component code (lines 215-261 of `Home/index.jsx`) MUST be completely removed.

**Rationale**: Eliminates duplicate FAQ management systems and ensures single source of truth.

**Priority**: High

#### Scenario: Hardcoded FAQ content is removed from homepage component

**Given** the homepage component source code (`web/src/pages/Home/index.jsx`)
**When** reviewing the component implementation
**Then** there should be no hardcoded FAQ question/answer arrays
**And** all FAQ content should be sourced from `StatusContext`

## MODIFIED Requirements

### Requirement: Homepage FAQ rendering MUST maintain design consistency

The visual design, styling, and responsive behavior of the FAQ section MUST remain identical to the current implementation when FAQ data is displayed.

**Rationale**: Ensures no visual regression or user experience degradation.

**Priority**: Medium

#### Scenario: FAQ cards maintain current styling

**Given** FAQ data is available and enabled
**When** FAQ items are rendered on the homepage
**Then** each FAQ card should use the same CSS classes as before:
- Background: `bg-semi-color-bg-1`
- Border radius: `rounded-2xl`
- Padding: `p-6`
- Shadow: `shadow-sm hover:shadow-md transition-shadow`

#### Scenario: FAQ section maintains responsive layout

**Given** FAQ data is available and enabled
**When** viewing the homepage on different screen sizes
**Then** the FAQ section should maintain responsive breakpoints (md, lg)
**And** question text should use responsive font sizes (`text-lg md:text-xl`)
**And** spacing should adapt properly on mobile devices

#### Scenario: FAQ section positioning remains unchanged

**Given** FAQ data is available and enabled
**When** the homepage is rendered
**Then** the FAQ section should appear in the same location relative to other homepage elements
**And** should use the same margin spacing (`mt-12 md:mt-16 lg:mt-20`)

## NOTES

### Data Structure Compatibility

The FAQ data structure from backend matches the frontend rendering needs:

```typescript
interface FAQItem {
  id: number;
  question: string;
  answer: string;
}
```

Frontend access via StatusContext:
```javascript
const faqData = statusState?.status?.faq || [];
const faqEnabled = statusState?.status?.faq_enabled ?? true;
```

### Related Capabilities

- **Console Settings Management**: FAQ data is stored and managed via `console_setting.faq` option
- **Admin FAQ Panel**: Existing admin panel at `web/src/pages/Setting/Dashboard/SettingsFAQ.jsx` manages FAQ CRUD operations
- **Dashboard FAQ Display**: Dashboard already displays FAQ data from backend (can be used as reference)

### Migration Considerations

Administrators will need to populate FAQ data through the admin panel after this change is deployed. The system should handle gracefully when FAQ data is empty (hide the section).

### Backward Compatibility

- ✅ No breaking changes to API
- ✅ No database schema changes
- ✅ StatusContext already provides FAQ data
- ✅ Existing FAQ admin panel continues to work unchanged
