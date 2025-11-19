# Spec: Rename Tutorial Navigation Label

## MODIFIED Requirements

### Requirement: Navigation SHALL Accurately Describe Tutorial Page Content

**Why:** Navigation labels SHALL accurately describe page content. The current "使用教程" label MUST be changed to "安装教程" because the page contains installation guides, not usage instructions.

#### Scenario: User views top navigation menu

**Given** a user is viewing any page on the site
**And** the top navigation bar is visible

**When** the user looks at the navigation menu items

**Then** the menu must display "安装教程" (not "使用教程")
**And** clicking "安装教程" must navigate to `/tutorial`
**And** the navigation item must highlight when user is on `/tutorial` page

#### Scenario: User switches between languages

**Given** a user is viewing the navigation in Chinese (`zh`)
**When** user switches language to English (`en`)
**Then** the navigation label must display "Installation Tutorial" (English translation)
**And** when user switches back to Chinese
**Then** the label must display "安装教程"

#### Scenario: Tutorial page metadata reflects new name

**Given** a user navigates to `/tutorial` page
**When** the page loads
**Then** the page title (`<title>` tag) should include "安装教程"
**And** any breadcrumbs should show "安装教程"
**And** the main heading on the page should remain "📚 AI Code 使用教程" (unchanged, as this is more comprehensive than just navigation label)

---

**Validation:**
- [x] Navigation label changed from "使用教程" to "安装教程" in `useNavigation.js`
- [x] i18n translation keys updated in `zh.json` and `en.json`
- [x] English translation added ("Installation Tutorial")
- [x] Navigation highlighting still works on `/tutorial` route
- [x] No broken links or navigation errors
