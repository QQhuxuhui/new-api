# Tasks: Add Inline Tutorial Page

## Phase 1: Preparation and Planning (✓ Complete)
- [x] Analyze claude-relay-service tutorial implementation
- [x] Review new-api routing structure and API endpoints
- [x] Identify UI components and styling patterns to use
- [x] Create OpenSpec proposal and task breakdown

## Phase 2: Frontend Implementation

### 2.1 Create Tutorial Page Component
- [ ] Create `/web/src/pages/Tutorial/index.jsx` page component
- [ ] Implement OS platform selection tabs (Windows, macOS, Linux/WSL2)
- [ ] Create dynamic URL generation utility function
- [ ] Build tutorial content sections:
  - [ ] Node.js installation guide (per OS)
  - [ ] Claude Code installation and configuration
  - [ ] OpenAI Codex installation and configuration
- [ ] Add code block components with syntax highlighting
- [ ] Implement "Copy to clipboard" functionality for code examples

### 2.2 Routing Configuration
- [ ] Add `/tutorial` route in `web/src/App.jsx`
- [ ] Configure route as publicly accessible (no authentication required)
- [ ] Add lazy loading for tutorial page component

### 2.3 Navigation Integration
- [ ] Update `web/src/hooks/common/useNavigation.js` to include tutorial link
- [ ] Add tutorial menu item to main navigation
- [ ] Update header navigation modules configuration (if needed)
- [ ] Ensure tutorial link shows in both authenticated and unauthenticated states

### 2.4 Internationalization (i18n)
- [ ] Add Chinese translation keys to `web/src/i18n/locales/zh.json`
- [ ] Add English translation keys to `web/src/i18n/locales/en.json`
- [ ] Add French translation keys to `web/src/i18n/locales/fr.json`
- [ ] Add Japanese translation keys to `web/src/i18n/locales/ja.json`
- [ ] Add Russian translation keys to `web/src/i18n/locales/ru.json`
- [ ] Test language switching on tutorial page

### 2.5 Styling and Responsive Design
- [ ] Apply Semi Design components (Card, Tabs, Typography, Button)
- [ ] Add Tailwind CSS utility classes for layout
- [ ] Ensure responsive design for mobile (breakpoints: sm, md, lg, xl)
- [ ] Add dark mode support consistent with platform theme
- [ ] Test on various screen sizes (mobile, tablet, desktop)

## Phase 3: Dynamic Configuration

### 3.1 URL Generation Logic
- [ ] Create utility function to get base URL (based on `window.location.origin`)
- [ ] Implement Claude Code endpoint URL generator (`{base}/v1`)
- [ ] Implement OpenAI Codex endpoint URL generator (`{base}/v1`)
- [ ] Handle edge cases (custom ports, subdomains, https/http)

### 3.2 Configuration Templates
- [ ] Generate Claude Code environment variable setup examples
- [ ] Generate OpenAI Codex .env file examples
- [ ] Include platform-specific configuration instructions (Windows vs Unix-like)
- [ ] Add API key placeholder examples (e.g., `sk-xxxxx`)

## Phase 4: Testing and Validation

### 4.1 Functional Testing
- [ ] Verify tutorial page loads at `/tutorial` route
- [ ] Test OS platform tab switching (Windows, macOS, Linux)
- [ ] Verify dynamic URL generation displays correct values
- [ ] Test "Copy to clipboard" functionality for all code blocks
- [ ] Ensure all links and navigation work correctly

### 4.2 Cross-Browser Testing
- [ ] Test on Chrome/Chromium
- [ ] Test on Firefox
- [ ] Test on Safari
- [ ] Test on Edge
- [ ] Verify mobile browser compatibility

### 4.3 Responsive Design Testing
- [ ] Test on mobile devices (320px - 480px width)
- [ ] Test on tablets (481px - 768px width)
- [ ] Test on desktop (769px+ width)
- [ ] Verify all content is readable and accessible at different sizes

### 4.4 Accessibility Testing
- [ ] Verify semantic HTML structure
- [ ] Test keyboard navigation
- [ ] Check color contrast ratios (WCAG AA compliance)
- [ ] Verify screen reader compatibility
- [ ] Add appropriate ARIA labels where needed

### 4.5 i18n Testing
- [ ] Test Chinese (zh) language display
- [ ] Test English (en) language display
- [ ] Test French (fr) language display
- [ ] Test Japanese (ja) language display
- [ ] Test Russian (ru) language display
- [ ] Verify dynamic content translates correctly

## Phase 5: Documentation and Cleanup

### 5.1 Code Documentation
- [ ] Add JSDoc comments to utility functions
- [ ] Document component props and usage
- [ ] Add inline code comments for complex logic

### 5.2 Update Project Documentation
- [ ] Update `docs/` folder with tutorial feature description (if applicable)
- [ ] Add tutorial page to relevant user guides
- [ ] Document URL generation logic for future reference

### 5.3 Code Review and Refinement
- [ ] Self-review code for quality and consistency
- [ ] Ensure code follows project conventions (React, Tailwind, Semi Design)
- [ ] Optimize bundle size (lazy loading, code splitting)
- [ ] Remove debug code and console.log statements

## Phase 6: Deployment Preparation

### 6.1 Build Validation
- [ ] Run production build: `cd web && npm run build`
- [ ] Verify no build errors or warnings
- [ ] Test built version locally
- [ ] Check bundle size impact

### 6.2 Final Validation
- [ ] Run OpenSpec validation: `openspec validate add-inline-tutorial-page --strict`
- [ ] Resolve any validation issues
- [ ] Ensure all tasks are completed
- [ ] Verify success criteria from proposal

---

**Progress**: 4/93 tasks completed
**Status**: In Progress
**Estimated Completion**: 4-5 days
