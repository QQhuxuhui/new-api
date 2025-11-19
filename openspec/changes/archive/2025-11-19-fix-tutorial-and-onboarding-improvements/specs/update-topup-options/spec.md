# Spec: Update Top-Up Options in Onboarding

## MODIFIED Requirements

### Requirement: TopupStep SHALL Show Only Available Payment Methods

**Why:** TopupStep SHALL show only available payment methods. The "在线支付" option MUST be removed because the system no longer supports online payment. Only valid methods (redemption codes and admin/Xianyu contact) SHALL be displayed.

#### Scenario: User views top-up options in onboarding wizard

**Given** a user is completing the onboarding wizard
**And** user reaches step 2 (充值账户 / Top-up Account)

**When** the TopupStep component renders

**Then** the UI must display exactly TWO option cards:
1. **兑换码充值** (Redemption Code Top-Up)
   - Icon: `IconTicketCodeStroked`
   - Input field for redemption code
   - "兑换" button
   - Existing API call to `POST /api/user/topup` (unchanged)

2. **联系管理员或闲鱼店铺购买** (Contact Admin or Xianyu Shop Purchase)
   - Icon: `IconUserCardPhone` or `IconShoppingBag`
   - Description: "如需帮助，请联系平台管理员或访问闲鱼店铺购买"
   - Two action buttons:
     - "联系管理员" - shows admin contact info (existing functionality)
     - "闲鱼店铺" - opens Xianyu shop link in new tab (if configured)

**And** the UI must NOT display:
- ❌ "在线支付" card
- ❌ "前往支付页面" button
- ❌ "我已完成充值" button

#### Scenario: Admin has configured Xianyu shop link

**Given** system administrator has configured `xianyu_shop_link` in backend settings
**And** user is viewing TopupStep in onboarding wizard

**When** the "联系管理员或闲鱼店铺购买" card is displayed

**Then** the "闲鱼店铺" button must be visible and enabled
**And** clicking the button must open the configured Xianyu URL in a new browser tab
**And** the URL must be validated (starts with `http://` or `https://`)

**Given** system administrator has NOT configured `xianyu_shop_link`
**When** the "联系管理员或闲鱼店铺购买" card is displayed
**Then** the "闲鱼店铺" button must be hidden or disabled
**And** only "联系管理员" button must be shown

#### Scenario: User successfully redeems a code

**Given** a user enters a valid redemption code
**And** clicks "兑换" button

**When** the API call succeeds

**Then** success message must display: "兑换成功! 获得额度: {amount}"
**And** user quota in context must be updated
**And** analytics event `onboarding_redemption_code_used` must be fired
**And** wizard must automatically advance to step 3 after 1 second delay
**And** redemption code input must be cleared

**Given** user enters an invalid redemption code
**When** the API call fails
**Then** error message must display (from API response)
**And** user must remain on step 2 (not auto-advance)
**And** user can retry with a different code

---

## ADDED Requirements

### Requirement: Backend SHALL Support Xianyu Shop Link Configuration

**Why:** Backend SHALL support xianyu_shop_link configuration. A separate configuration field MUST be added to allow administrators to configure Xianyu shop links independently from other payment settings.

#### Scenario: Admin configures Xianyu shop link in backend

**Given** an administrator accesses system settings
**When** admin navigates to payment/top-up configuration section

**Then** a new configuration field `xianyu_shop_link` must be available
**And** field must accept a valid URL (validation: starts with `http://` or `https://`)
**And** field must be optional (can be left empty)
**And** field must have max length of 512 characters
**And** field must have help text: "闲鱼店铺购买链接（可选），留空则不显示闲鱼店铺按钮"

#### Scenario: Frontend retrieves Xianyu shop link

**Given** backend has Xianyu shop link configured
**When** frontend loads status configuration via `/api/status` endpoint

**Then** response must include `xianyu_shop_link` field in status object
**And** value must be accessible via `statusState?.status?.xianyu_shop_link`
**And** value must be empty string if not configured (not null/undefined)

---

**Validation:**
- [x] "在线支付" option card removed from TopupStep UI
- [x] "联系管理员或闲鱼店铺购买" combined option card added
- [x] Xianyu shop button opens link in new tab (when configured)
- [x] Xianyu shop button hidden when link not configured
- [x] Backend configuration field `xianyu_shop_link` added
- [x] Status API returns Xianyu link configuration
- [x] Redemption code functionality unchanged and working
- [x] Step navigation still works correctly (skip, back, next)
- [x] Analytics tracking maintained
- [x] Responsive design works on mobile
