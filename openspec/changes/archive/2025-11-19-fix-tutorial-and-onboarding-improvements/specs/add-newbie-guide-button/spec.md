# Spec: Add Newbie Guide Button

## ADDED Requirements

### Requirement: Top Navigation SHALL Provide Access to Onboarding Wizard

**Why:** The system SHALL provide manual access to OnboardingWizard. A "新手指引" button MUST be added to navigation because users currently have no way to manually access the wizard after first login.

#### Scenario: User clicks newbie guide button in navigation

**Given** a user is logged in and viewing any page
**And** the top navigation bar is visible

**When** user clicks "新手指引" button in the navigation

**Then** the `OnboardingWizard` modal must open
**And** the wizard must start from step 1 (Welcome screen)
**And** the user's previous onboarding progress (if any) must be loaded from localStorage
**And** the wizard must allow user to navigate through all steps

#### Scenario: Navigation button is visible to all users

**Given** a user is logged in
**When** the user views the top navigation
**Then** the "新手指引" button must be visible
**And** the button must be accessible via keyboard navigation (Tab key)
**And** the button must work on mobile responsive layout

**Given** a user is NOT logged in
**When** the user views the top navigation
**Then** the "新手指引" button must also be visible
**And** clicking it must redirect to `/login` page (since onboarding requires authentication)

---

### Requirement: Homepage Tutorial Button SHALL Conditionally Open Wizard

**Why:** The homepage "使用教程" button SHALL conditionally open the onboarding wizard. It MUST open OnboardingWizard for logged-in users and redirect to /login for non-logged-in users.

#### Scenario: Logged-in user clicks tutorial button on homepage

**Given** a user is logged in
**And** user is viewing the homepage (`/`)

**When** user clicks the "使用教程" button (icon: `IconFile`)

**Then** the `OnboardingWizard` modal must open
**And** the user must NOT be redirected to another page
**And** the wizard must function identically to when opened from navigation

#### Scenario: Non-logged-in user clicks tutorial button on homepage

**Given** a user is NOT logged in
**And** user is viewing the homepage (`/`)

**When** user clicks the "使用教程" button

**Then** the browser must navigate to `/login` page
**And** after successful login, user should be redirected back to homepage (standard login flow)

#### Scenario: Analytics tracking for button sources

**Given** a user opens the onboarding wizard
**When** the wizard is opened from navigation button
**Then** analytics event `newbie_guide_opened` must be fired with `source: "nav_button"`

**When** the wizard is opened from homepage button
**Then** analytics event `newbie_guide_opened` must be fired with `source: "homepage_button"`

---

**Validation:**
- [x] "新手指引" button added to top navigation component
- [x] Button opens `OnboardingWizard` for logged-in users
- [x] Button redirects to `/login` for non-logged-in users
- [x] Homepage "使用教程" button opens wizard for logged-in users
- [x] Homepage "使用教程" button redirects to login for non-logged-in users
- [x] Analytics events track button sources correctly
- [x] Wizard state persistence works across different entry points
- [x] Responsive design works on mobile devices
- [x] Keyboard navigation and accessibility maintained
