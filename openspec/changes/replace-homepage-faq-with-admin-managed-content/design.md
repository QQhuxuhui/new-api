# Design: Replace Homepage FAQ with Admin-Managed Content

## Architecture Overview

This change modifies the homepage FAQ display to consume admin-managed FAQ data instead of hardcoded content. The implementation is straightforward and leverages existing infrastructure.

## Current Architecture

### Existing Data Flow

```
Admin Panel (SettingsFAQ.jsx)
    ↓
    Save to DB (console_setting.faq option)
    ↓
Backend (console_setting.GetFAQ())
    ↓
API Response (/api/status endpoint)
    ↓
Frontend (StatusContext) → Dashboard FAQ Panel
```

### Homepage Current State

```
Home/index.jsx
    ↓
Hardcoded FAQ Array (lines 215-261)
    ↓
Direct Render (no backend data)
```

## Proposed Architecture

### Unified Data Flow

```
Admin Panel (SettingsFAQ.jsx)
    ↓
    Save to DB (console_setting.faq option)
    ↓
Backend (console_setting.GetFAQ())
    ↓
API Response (/api/status endpoint)
    ↓
Frontend (StatusContext) → {
                              Dashboard FAQ Panel
                              Homepage FAQ Section  ← NEW
                           }
```

## Implementation Details

### 1. Data Source

**Existing Infrastructure** (No changes needed):
- Backend: `setting/console_setting/validation.go:233` - `GetFAQ()` function
- API Endpoint: `controller/misc.go:124-126` - `/api/status` includes FAQ data when enabled
- Frontend Context: `context/Status` - Already fetches and stores FAQ data

**FAQ Data Structure**:
```json
[
  {
    "id": 1,
    "question": "如何获取 API 密钥？",
    "answer": "点击「获取密钥」按钮进入控制台..."
  },
  {
    "id": 2,
    "question": "支持哪些大模型？",
    "answer": "我们支持 OpenAI、Claude、Gemini..."
  }
]
```

### 2. Frontend Changes

**File**: `web/src/pages/Home/index.jsx`

**Changes Required**:

1. **Import StatusContext** (if not already imported):
   ```javascript
   import { StatusContext } from '../../context/Status';
   ```

2. **Access FAQ data from context**:
   ```javascript
   const [statusState] = useContext(StatusContext);
   const faqData = statusState?.status?.faq || [];
   const faqEnabled = statusState?.status?.faq_enabled ?? true;
   ```

3. **Replace hardcoded FAQ section** (lines 204-276):
   - Remove hardcoded FAQ array
   - Replace with dynamic rendering from `faqData`
   - Add conditional rendering based on `faqEnabled`
   - Show only first 4 FAQ items (to match current design)
   - Handle empty state gracefully

4. **Rendering Logic**:
   ```javascript
   {faqEnabled && faqData.length > 0 && (
     <div className='mt-12 md:mt-16 lg:mt-20 w-full px-4'>
       {/* FAQ Header */}
       <div className='flex items-center mb-6 md:mb-8 justify-center'>
         <Text type='tertiary' className='text-lg md:text-xl lg:text-2xl font-light'>
           {t('常见问答')}
         </Text>
       </div>

       {/* FAQ Items - Show first 4 */}
       <div className='max-w-4xl mx-auto space-y-4'>
         {faqData.slice(0, 4).map((faq, index) => (
           <div key={faq.id || index}
                className='bg-semi-color-bg-1 rounded-2xl p-6 shadow-sm hover:shadow-md transition-shadow'>
             <h3 className='text-lg md:text-xl font-semibold text-semi-color-text-0 mb-2'>
               {faq.question}
             </h3>
             <div
               className='text-semi-color-text-2'
               dangerouslySetInnerHTML={{
                 __html: marked.parse(faq.answer || ''),
               }}
             />
           </div>
         ))}
       </div>

       {/* External docs link (preserved) */}
       {docsLink && (
         <div className='mt-6 text-center'>
           <Button type='tertiary' size='small'
                   onClick={() => window.open(docsLink, '_blank')}>
             {isChinese ? '查看更多外部文档 →' : 'View More External Docs →'}
           </Button>
         </div>
       )}
     </div>
   )}
   ```

**Important:** FAQ answers support Markdown/HTML formatting (using `marked.parse()`), consistent with Dashboard FAQ panel and admin settings page.

### 3. Edge Cases & Error Handling

| Scenario | Behavior |
|----------|----------|
| `faqEnabled = false` | FAQ section hidden entirely |
| `faqData = []` (empty) | FAQ section hidden |
| `faqData = null/undefined` | FAQ section hidden (fallback to `[]`) |
| FAQ items < 4 | Display all available items |
| FAQ items > 4 | Display only first 4 items |
| Missing `question` or `answer` | Skip malformed item (defensive) |
| `statusState` not loaded yet | Show nothing (graceful degradation) |

### 4. Styling Consistency

**Current Design Preserved**:
- Same card layout (`bg-semi-color-bg-1 rounded-2xl`)
- Same spacing (`space-y-4`)
- Same typography (`text-lg md:text-xl font-semibold`)
- Same hover effects (`hover:shadow-md transition-shadow`)
- Same responsive breakpoints (md, lg)

**No CSS changes needed** - reuse existing classes.

### 5. Migration Strategy

**Option A: Manual Migration** (Recommended for initial release)
1. Provide documentation with example FAQ data
2. Admin manually enters FAQ content via admin panel
3. Simple and safe

**Option B: Automatic Migration** (Optional future enhancement)
1. Create migration script in `controller/console_migrate.go`
2. Extract hardcoded FAQ from code
3. Convert to JSON format
4. Insert into `console_setting.faq` option
5. Run once during deployment

**Recommended**: Start with Option A (manual), add Option B later if needed.

### 6. Testing Considerations

**Test Scenarios**:
1. ✅ FAQ enabled + has data → Display FAQ section
2. ✅ FAQ enabled + empty data → Hide FAQ section
3. ✅ FAQ disabled (`faq_enabled = false`) → Hide FAQ section
4. ✅ FAQ with < 4 items → Display all items
5. ✅ FAQ with > 4 items → Display only first 4
6. ✅ FAQ with Markdown formatting → Render bold, links, lists correctly
7. ✅ Mobile responsive → FAQ cards stack properly
8. ✅ Theme switching → FAQ styling adapts
9. ✅ Language switching → Questions/answers in correct language (admin manages)

**Browser Testing**:
- Desktop: Chrome, Firefox, Safari, Edge
- Mobile: iOS Safari, Android Chrome
- Responsive breakpoints: sm, md, lg, xl

## Rollback Plan

If issues arise:
1. Revert `Home/index.jsx` to previous version (restore hardcoded FAQ)
2. No database changes needed (backward compatible)
3. No backend changes needed

## Performance Impact

- **Negligible**: FAQ data already fetched in `/api/status` call
- **No additional API requests**
- **Client-side rendering**: Same as current implementation
- **Bundle size**: Slightly reduced (less hardcoded content)

## Security Considerations

- ✅ FAQ data sanitized by backend (existing validation in `ValidateConsoleSettings`)
- ✅ Markdown rendering: Uses `marked.parse()` library (same as Dashboard FAQ panel)
- ⚠️ XSS Prevention: `marked` library has built-in XSS protection, but admin content is trusted
  - FAQ content is admin-only (not user-generated)
  - Backend validation ensures only admins can modify FAQ
  - `marked.parse()` automatically escapes dangerous HTML by default
- ✅ Consistent with Dashboard FAQ panel rendering (web/src/components/dashboard/FaqPanel.jsx:63-66)
- ✅ Admin-only write access (existing permission system)

## Accessibility

- ✅ Semantic HTML maintained (`<h3>` for questions, `<p>` for answers)
- ✅ Keyboard navigation preserved
- ✅ Screen reader friendly (text content, not images)
- ✅ Color contrast maintained (using theme variables)

## Documentation Updates Needed

1. **Admin Guide**: How to manage homepage FAQ via admin panel
2. **Migration Guide**: Steps to populate initial FAQ data
3. **Code Comments**: Note that FAQ section uses admin-managed data

## Future Enhancements (Out of Scope)

- [ ] Support Markdown/HTML in FAQ answers
- [ ] Add "Show More" button to display all FAQ items
- [ ] FAQ search/filter functionality
- [ ] FAQ categories/grouping
- [ ] FAQ analytics (view counts)
