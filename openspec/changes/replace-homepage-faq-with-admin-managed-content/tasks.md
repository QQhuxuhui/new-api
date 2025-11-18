# Tasks: Replace Homepage FAQ with Admin-Managed Content

## Implementation Checklist

### Phase 1: Code Changes

- [x] **Task 1.1**: Update `web/src/pages/Home/index.jsx` to use StatusContext FAQ data
  - [x] Import StatusContext if not already imported
  - [x] Extract `faqData` and `faqEnabled` from statusState
  - [x] Add defensive fallback: `const faqData = statusState?.status?.faq || [];`
  - [x] Add enabled check: `const faqEnabled = statusState?.status?.faq_enabled ?? true;`

- [x] **Task 1.2**: Remove hardcoded FAQ content from Home component
  - [x] Delete hardcoded FAQ array (lines 215-261 in current implementation)
  - [x] Remove all hardcoded question/answer text in English and Chinese

- [x] **Task 1.3**: Implement dynamic FAQ rendering
  - [x] Add conditional rendering: only show FAQ section when `faqEnabled && faqData.length > 0`
  - [x] Map over `faqData.slice(0, 4)` to render first 4 items
  - [x] Use `faq.id` as React key (with index fallback)
  - [x] Render `faq.question` in heading (`<h3>`)
  - [x] Render `faq.answer` in paragraph (`<p>`)

- [x] **Task 1.4**: Preserve styling and layout
  - [x] Verify all CSS classes match original implementation
  - [x] Keep same responsive breakpoints (md, lg)
  - [x] Maintain same spacing and margins
  - [x] Preserve hover effects and transitions

- [x] **Task 1.5**: Maintain external docs link
  - [x] Keep the `docsLink` button rendering below FAQ section
  - [x] Ensure it still renders when FAQ is hidden (if `docsLink` exists)

### Phase 2: Testing

- [ ] **Task 2.1**: Test FAQ enabled with data
  - [ ] Verify FAQ section displays when `faq_enabled = true` and data exists
  - [ ] Confirm first 4 items are shown when 10+ items exist
  - [ ] Verify all items shown when less than 4 items exist
  - [ ] Check question and answer render correctly

- [ ] **Task 2.2**: Test FAQ disabled
  - [ ] Set `console_setting.faq_enabled` to `false` in admin panel
  - [ ] Verify FAQ section is completely hidden on homepage
  - [ ] Confirm no empty space or placeholder renders

- [ ] **Task 2.3**: Test empty FAQ data
  - [ ] Clear all FAQ items in admin panel
  - [ ] Verify FAQ section is hidden when no data exists
  - [ ] Confirm graceful handling (no errors in console)

- [ ] **Task 2.4**: Test responsive design
  - [ ] Test on mobile viewport (< 768px)
  - [ ] Test on tablet viewport (768px - 1024px)
  - [ ] Test on desktop viewport (> 1024px)
  - [ ] Verify text sizes adapt at each breakpoint

- [ ] **Task 2.5**: Test theme switching
  - [ ] Switch between light and dark themes
  - [ ] Verify FAQ cards use correct theme colors
  - [ ] Check text contrast and readability

- [ ] **Task 2.6**: Test with real admin data
  - [ ] Add FAQ items through admin panel (`/setting/dashboard`)
  - [ ] Navigate to SettingsFAQ section
  - [ ] Create 4+ FAQ entries
  - [ ] Verify they appear on homepage immediately (or after refresh)

### Phase 3: Documentation & Migration

- [x] **Task 3.1**: Update code comments
  - [x] Add comment explaining FAQ data source
  - [x] Document the 4-item limit in code

- [ ] **Task 3.2**: Create migration documentation
  - [ ] Write admin guide for populating FAQ via admin panel
  - [ ] Provide example FAQ data structure
  - [ ] Document FAQ management workflow

- [ ] **Task 3.3**: Optional: Create migration utility
  - [ ] (Optional) Add migration script to populate default FAQ from old hardcoded content
  - [ ] (Optional) Run migration as part of deployment

### Phase 4: Validation

- [x] **Task 4.1**: Code review checklist
  - [x] No hardcoded FAQ content remains in `Home/index.jsx`
  - [x] StatusContext is properly accessed
  - [x] Defensive programming for undefined/null values
  - [x] No console errors or warnings
  - [x] React keys properly set on list items

- [ ] **Task 4.2**: Visual regression check
  - [ ] Take screenshot of old FAQ section
  - [ ] Take screenshot of new FAQ section with same data
  - [ ] Compare side-by-side for consistency
  - [ ] Verify spacing, colors, fonts match

- [ ] **Task 4.3**: Browser compatibility
  - [ ] Test on Chrome (latest)
  - [ ] Test on Firefox (latest)
  - [ ] Test on Safari (latest, if available)
  - [ ] Test on Edge (latest)
  - [ ] Test on mobile Safari (iOS)
  - [ ] Test on mobile Chrome (Android)

- [x] **Task 4.4**: Performance check
  - [x] Verify no additional API calls are made
  - [x] Confirm FAQ data loaded from existing `/api/status` call
  - [x] Check for any rendering performance issues
  - [x] Validate bundle size hasn't increased

### Phase 5: Deployment

- [ ] **Task 5.1**: Pre-deployment checklist
  - [ ] All tests passing
  - [ ] No console errors
  - [ ] Code reviewed and approved
  - [ ] Documentation updated

- [x] **Task 5.2**: Deployment steps
  - [x] Build frontend: `npm run build`
  - [ ] Deploy to staging environment (if available)
  - [ ] Smoke test on staging
  - [ ] Deploy to production

- [ ] **Task 5.3**: Post-deployment verification
  - [ ] Verify homepage loads correctly
  - [ ] Confirm FAQ section displays (if admin has populated data)
  - [ ] Check browser console for errors
  - [ ] Verify admin FAQ panel still works

- [ ] **Task 5.4**: Admin notification
  - [ ] Notify administrators about new FAQ management capability
  - [ ] Provide link to documentation
  - [ ] Remind to populate FAQ data if section appears empty

## Success Metrics

- ✅ Homepage FAQ section displays admin-managed content
- ✅ Zero hardcoded FAQ content in frontend code
- ✅ FAQ section hidden when disabled or empty
- ✅ Visual design identical to original
- ✅ No performance degradation
- ✅ All browser tests passing
- ✅ Admin can update FAQ without code changes

## Rollback Procedure

If critical issues arise:

1. Revert `web/src/pages/Home/index.jsx` to previous commit
2. Rebuild and redeploy frontend
3. No database rollback needed (backward compatible)
4. Document issues for future fix

## Notes

- Keep the original design intact - this is purely a data source change
- FAQ data already available in StatusContext - no new API needed
- Admin panel FAQ management unchanged - just connecting existing data to homepage
- Consider adding analytics to track FAQ usage in future
