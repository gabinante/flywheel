# GoHighPayment — Project Specification

## Business Model

GoHighPayment is an **NMI ISO referral partner** platform. We refer merchants to NMI, NMI underwrites/settles directly to merchants, and we earn residuals from NMI on processing volume. We **never hold, transmit, or settle merchant funds**.

Our platform layers a viral affiliate engine on top: GHL agencies sign up merchants through us, we board those merchants onto NMI via the partner boarding API, and we share a portion of the residual NMI pays us with the referring agency.

### Revenue Flow

```
Merchant processes $100,000/mo
  → NMI settles directly to merchant's bank account
  → NMI pays us ~X bps residual on that volume (per partner agreement)
  → We keep Y bps, pay referring agency Z bps from our cut
  → If agency was referred by another agency, referrer gets 3 bps from our cut
```

We are **not** a payment facilitator. We are **not** a money transmitter. NMI is the merchant of record for payment processing.

---

## Architecture Overview

### What NMI Provides (we configure, not build)
- Payment gateway API (authorize, capture, void, refund, recurring)
- Customer Vault (tokenized card storage, PCI Level 1)
- Collect.js / Payment Component (hosted card capture, zero PCI scope)
- Merchant Central (white-labeled partner portal)
- Residuals reporting (per-merchant earnings)
- Underwriting & KYC (NMI screens merchants via ScanX)
- Fraud tools (iSpyFraud + 3DS Payer Authentication 2.0)
- Network tokenization + automatic card updater
- Webhooks (transactions, settlements, disputes)
- Partner boarding API (automated merchant provisioning)

### What We Build
- GHL Custom Payment Provider integration (queryUrl, OAuth, SSO)
- Merchant onboarding wizard (submits to NMI boarding API)
- White-label checkout surfaces (hosted page, payment links, invoices)
- Agency/affiliate registration, attribution, and dashboard
- Residual tracking and agency rev-share accounting
- Merchant self-service portal (transactions, settings, webhooks)
- Admin/operator dashboard (portfolio view, chargebacks, analytics)
- Embeddable payment SDK

---

## Current State (What's Already Built)

### Database (Prisma/PostgreSQL)
- 21 models: Merchant, Customer, Transaction, SeamlesschexTransaction, Chargeback, AdminUser, AuditLog, WebhookEvent, MerchantWebhookDelivery, DailyMetrics, DeadLetterQueue, Application, NotificationSchedule, GhlOAuthState, CheckoutSession
- Per-merchant NMI credentials (nmiSecurityKey, nmiTokenizationKey)
- Per-merchant Seamlesschex credentials for ACH
- Payment methods: CARD, ACH, GOOGLE_PAY, APPLE_PAY

### Backend (Fastify + TypeScript)
- **Admin auth**: bootstrap, login, change-password, refresh
- **Admin routes**: merchants CRUD, transactions list/detail, chargebacks, staff management, analytics (daily + summary), audit log
- **Merchant auth**: register, login, refresh
- **Merchant routes**: dashboard stats, transactions + refunds, webhook config + delivery history, API key rotation, settings, 3-step onboarding (business info → owner/banking → NMI boarding submission)
- **Checkout routes**: session create, config (tokenization key), process card, process ACH
- **GHL routes**: OAuth install, callback, SSO, webhook (install/uninstall)
- **Webhook receivers**: NMI (sale, refund, chargeback), Seamlesschex (pending, cleared, settled, returned, voided)
- **Services**: NMI (charge, refund, void), Seamlesschex (create check, status, void), NMI Boarding (submit application, check status), Post-payment pipeline
- **Jobs**: pg-boss queue (webhook delivery, post-payment processing)

### Frontend Apps
- **Admin Dashboard** (React/Vite, port 5174): Login, Dashboard, Merchants, MerchantDetail, Transactions, Chargebacks, Staff, Analytics, AuditLog
- **Merchant Dashboard** (React/Vite, port 5175): Login, Dashboard, Transactions, Webhooks, Settings, Onboarding (3-step wizard)
- **Checkout** (React/Vite, port 5176): Card payment (Collect.js iframe), ACH (bank form), Confirmation page. Embeddable via iframe with postMessage.

### SDK
- `GoHighPayment` class: `redirect()` for full-page, `mount()` for iframe embed
- Session creation, postMessage event bus (success, error, cancel, resize)

### Infrastructure
- Docker Compose: PostgreSQL 16, Redis 7, all 4 services
- Fly.io deployment: separate fly.toml per service, deploy.sh script
- Makefile: dev, build, db, deploy targets
- Monorepo: npm workspaces, 5 packages

---

## What Needs to Be Built

### Work Stream 1: Security & Encryption Hardening

#### 1.1 — Field-Level Encryption for Sensitive Data
**Objective**: Encrypt all sensitive fields at rest in the database.

**Fields requiring encryption**:
- `Merchant.nmiSecurityKey`, `Merchant.nmiTokenizationKey`
- `Merchant.seamlesschexApiKey`
- `Application.ssnLast4`, `Application.bankRoutingNumber`, `Application.bankAccountNumber`
- Any future fields containing PII or credentials

**Implementation**:
- Use `ENCRYPTION_KEY` env var (already defined) with AES-256-GCM
- Create `packages/backend/src/utils/encryption.ts` with `encrypt(plaintext)` → `{iv, ciphertext, tag}` and `decrypt(encrypted)` → `plaintext`
- Store encrypted values as JSON string in existing text columns (no schema migration needed) or use a `_encrypted` suffix column pattern
- Add Prisma middleware or service-layer wrapper to auto-encrypt on write, auto-decrypt on read
- Key rotation support: store key version with each encrypted value, decrypt with old key + re-encrypt with new key during rotation

**Success criteria**:
- No plaintext credentials in database
- `ENCRYPTION_KEY` rotation procedure documented and tested
- All existing NMI service calls still work (decrypt before API call)

#### 1.2 — Webhook Signature Verification (Outbound)
**Objective**: Sign all webhook deliveries to merchants so they can verify authenticity.

**Implementation**:
- Generate per-merchant `webhookSecret` (random 32-byte hex) on merchant creation
- Include `X-GHP-Signature` header on all outbound webhook POSTs: `HMAC-SHA256(webhookSecret, rawBody)`
- Include `X-GHP-Timestamp` header (Unix epoch) to prevent replay attacks
- Document verification procedure for merchants in API docs

**Success criteria**:
- All outbound webhooks signed
- Merchant can verify signature with their secret
- Replay window limited to 5 minutes

#### 1.3 — Per-Merchant Rate Limiting
**Objective**: Prevent abuse by rate-limiting API calls per API key.

**Implementation**:
- Redis sliding window rate limiter keyed on API key
- Default: 100 requests/minute for checkout endpoints, 1000/minute for read endpoints
- Return `429 Too Many Requests` with `Retry-After` header
- Admin can override limits per merchant

**Success criteria**:
- Rate limits enforced per API key
- Global rate limit remains as backstop
- Limits configurable per merchant via admin

#### 1.4 — NMI Webhook Signature Verification (Inbound)
**Objective**: Validate all inbound NMI webhooks using their signing mechanism.

**Implementation**:
- NMI signs webhooks with a shared secret configured in the NMI partner portal
- Verify HMAC signature on every incoming NMI webhook before processing
- Reject unsigned or invalid-signature requests with 401
- Log all rejected attempts in audit log

**Success criteria**:
- No unsigned NMI webhooks processed
- Invalid signatures logged and rejected

---

### Work Stream 2: Affiliate & Agency Engine

#### 2.1 — Agency Data Model
**Objective**: Add agency/affiliate entities to the database.

**Schema additions (Prisma)**:
```prisma
model Agency {
  id                String    @id @default(cuid())
  name              String
  contactEmail      String    @unique
  contactPhone      String?
  passwordHash      String
  referralCode      String    @unique  // e.g., "agency-abc123"
  referredByAgencyId String?  // two-tier: who referred this agency
  referredByAgency  Agency?   @relation("AgencyReferrals", fields: [referredByAgencyId], references: [id])
  referredAgencies  Agency[]  @relation("AgencyReferrals")
  tier              AgencyTier @default(TIER_1)
  tierOverride      Boolean   @default(false) // manual tier lock
  status            AgencyStatus @default(ACTIVE)
  payoutEmail       String?
  payoutBankLast4   String?
  taxIdOnFile       Boolean   @default(false)  // W-9 received
  merchants         Merchant[]
  residualEntries   ResidualEntry[]
  residualPayouts   ResidualPayout[]
  createdAt         DateTime  @default(now())
  updatedAt         DateTime  @updatedAt
}

enum AgencyTier {
  TIER_1  // 0-$1M/mo portfolio vol, 10 bps
  TIER_2  // $1M-$5M/mo, 12 bps
  TIER_3  // $5M+/mo, 15 bps
}

enum AgencyStatus {
  ACTIVE
  SUSPENDED
  CHURNED
}
```

**Merchant model additions**:
```prisma
model Merchant {
  // ... existing fields ...
  agencyId          String?
  agency            Agency?   @relation(fields: [agencyId], references: [id])
  attributedAt      DateTime? // immutable once set
}
```

**Success criteria**:
- Migration runs cleanly against existing data
- Existing merchants have `agencyId = null` (pre-affiliate era)
- `referralCode` unique index enforced
- Agency-merchant attribution is immutable (no update path for `agencyId` once set)

#### 2.2 — Agency Registration & Authentication
**Objective**: Agencies can sign up, log in, and manage their account.

**Endpoints**:
- `POST /api/v1/agency/auth/register` — name, email, password, optional `ref` (referral code of referring agency)
- `POST /api/v1/agency/auth/login` — returns JWT (15m) + refresh token (7d)
- `POST /api/v1/agency/auth/refresh`
- `GET /api/v1/agency/profile` — agency details
- `PATCH /api/v1/agency/profile` — update contact info, payout details

**Registration logic**:
1. Validate input (Zod schema)
2. If `ref` provided, look up referring agency by `referralCode`. Set `referredByAgencyId`. (If invalid ref code, register anyway — don't block registration over a bad referral link.)
3. Generate unique `referralCode` for new agency
4. Hash password with bcrypt
5. Create agency record
6. Return JWT

**Success criteria**:
- Agency can register with or without referral code
- Two-tier relationship recorded immutably on registration
- JWT auth works identically to merchant auth pattern
- Referral code displayed in agency profile

#### 2.3 — Affiliate Referral Link & Attribution
**Objective**: When a merchant signs up through an agency's link, attribute permanently.

**Implementation**:
- Agency shares link: `https://app.gohighpayment.com/signup?ref={referralCode}`
- On merchant registration or GHL OAuth install, check for `ref` query param
- If present, look up agency by `referralCode`
- Set `merchant.agencyId = agency.id` and `merchant.attributedAt = now()`
- **Attribution is immutable**: once set, `agencyId` cannot be changed via any endpoint
- Also support cookie-based attribution: if `ref` param present on any page visit, store in cookie (90-day expiry). On merchant signup, check cookie if no `ref` in URL.

**Changes to existing code**:
- `POST /api/v1/merchant/auth/register` — accept optional `ref` param
- `GET /api/v1/ghl/install` — pass `ref` param through OAuth state
- `GET /api/v1/ghl/callback` — extract `ref` from state, attribute merchant

**Success criteria**:
- Merchants attributed to agencies on signup
- Attribution survives the GHL OAuth flow
- Cookie fallback works for delayed signups
- No endpoint allows changing attribution after set

#### 2.4 — Residual Tracking & Ledger
**Objective**: Track what NMI pays us per merchant and calculate agency share.

**Schema**:
```prisma
model ResidualEntry {
  id                String    @id @default(cuid())
  agencyId          String
  agency            Agency    @relation(fields: [agencyId], references: [id])
  merchantId        String
  merchant          Merchant  @relation(fields: [merchantId], references: [id])
  periodStart       DateTime  // first of month
  periodEnd         DateTime  // last of month
  merchantVolume    Int       // cents — total processing volume for this merchant this month
  nmiResidualEarned Int       // cents — what NMI paid us for this merchant
  agencyBps         Int       // basis points at time of calculation (10, 12, or 15)
  agencyShare       Int       // cents — agency's cut of our residual
  twoTierAgencyId   String?   // if this agency was referred, the referring agency
  twoTierShare      Int       @default(0) // cents — 3 bps to referring agency
  status            ResidualStatus @default(PENDING)
  approvedAt        DateTime?
  approvedBy        String?   // admin user ID
  createdAt         DateTime  @default(now())
  updatedAt         DateTime  @updatedAt

  @@unique([agencyId, merchantId, periodStart])
}

model ResidualPayout {
  id                String    @id @default(cuid())
  agencyId          String
  agency            Agency    @relation(fields: [agencyId], references: [id])
  periodStart       DateTime
  totalAmount       Int       // cents
  directShare       Int       // cents — from own merchants
  twoTierShare      Int       // cents — from referred agencies' merchants
  method            String    // "manual", "ach", etc.
  reference         String?   // check number, ACH trace, etc.
  status            PayoutStatus @default(PENDING)
  paidAt            DateTime?
  createdAt         DateTime  @default(now())
  updatedAt         DateTime  @updatedAt
}

enum ResidualStatus {
  PENDING     // calculated, awaiting approval
  APPROVED    // operator approved
  PAID        // included in payout
  HELD        // operator held (e.g., chargeback issue)
}

enum PayoutStatus {
  PENDING
  APPROVED
  PAID
  FAILED
}
```

**Calculation job** (monthly, pg-boss scheduled):
1. Pull per-merchant processing volume from NMI Query API (`query.php`) for the period
2. For each merchant with an `agencyId`:
   a. Look up agency tier → determine bps rate
   b. Calculate `agencyShare = merchantVolume * agencyBps / 10000`
   c. If agency has `referredByAgencyId`, calculate `twoTierShare = merchantVolume * 3 / 10000`
   d. Create `ResidualEntry` with status `PENDING`
3. Log calculation run in audit log

**Important rules**:
- Residuals are calculated from **NMI's reported numbers**, not our transaction table. NMI is the source of truth for volume.
- Chargebacks reduce merchant volume in NMI's report, so they're automatically excluded.
- Residual entries are append-only. Never update a finalized entry — create an adjustment entry instead.

**Success criteria**:
- Monthly job runs and produces correct residual entries
- Two-tier referral earnings calculated correctly
- All entries auditable with immutable trail
- Operator can review before payout

#### 2.5 — Agency Dashboard (Frontend)
**Objective**: New React app for agency users.

**Package**: `packages/agency-dashboard` (React/Vite, port 5177)

**Pages**:
- **Login**: email/password auth
- **Dashboard**: portfolio overview — total active merchants, total volume this month, current tier, earned residual this month, next tier threshold + progress bar
- **Merchants**: table of attributed merchants — name, activation date, volume this month, residual earned. Sortable/filterable. Agency **cannot** see individual transaction details.
- **Network**: two-tier view — agencies referred by this agency, their portfolio volume, two-tier earnings per referred agency
- **Referrals**: display referral link + code, copy button, QR code generation (use `qrcode` npm package), pre-written share text
- **Payouts**: history of past monthly payouts — amount, date, method, status. CSV export for tax purposes.
- **Settings**: profile info, payout preferences, W-9 upload status

**Success criteria**:
- Agency can log in and see their portfolio
- Real-time (daily-refresh) residual tracking
- Referral link sharing works
- Payout history exportable

#### 2.6 — Volume Tier Auto-Upgrade Job
**Objective**: Monthly recalculation of agency tier based on portfolio volume.

**Implementation**:
- pg-boss scheduled job, runs 1st of each month after residual calculation
- For each agency where `tierOverride = false`:
  - Sum all attributed merchants' volume for the month
  - TIER_1: < $1M/mo
  - TIER_2: $1M–$5M/mo
  - TIER_3: $5M+/mo
- If tier changes: update agency, create audit log entry, queue notification email
- Tier changes take effect for next month's residual calculation

**Success criteria**:
- Tiers recalculate monthly
- Admin can lock a tier via `tierOverride`
- Tier changes audited

---

### Work Stream 3: Operator Admin Enhancements

#### 3.1 — Agency Management in Admin Dashboard
**Objective**: Admin can view and manage all agencies.

**Endpoints**:
- `GET /api/v1/admin/agencies` — list all agencies with pagination, filtering (tier, status, volume range)
- `GET /api/v1/admin/agencies/:id` — detail view with merchants, referral tree, residual history
- `PATCH /api/v1/admin/agencies/:id` — update status, tier override, suspend/reactivate

**Admin dashboard pages**:
- **Agencies list**: table with name, tier, status, merchant count, portfolio volume, monthly residual
- **Agency detail**: full profile, merchant list, referral network tree, residual history, payout history

**Success criteria**:
- Admin has full visibility into affiliate network
- Can suspend problematic agencies
- Can override tier assignments

#### 3.2 — Residual Approval Workflow
**Objective**: Operator reviews and approves residuals before payout.

**Endpoints**:
- `GET /api/v1/admin/residuals` — list residual entries for a period, filterable by status
- `POST /api/v1/admin/residuals/approve` — approve selected entries (batch)
- `POST /api/v1/admin/residuals/hold` — hold entries with reason
- `GET /api/v1/admin/residuals/summary` — period summary: total owed, by tier, by agency

**Admin dashboard page**:
- **Residuals**: period selector, table of entries grouped by agency, approve/hold actions, batch approve all, summary totals

**Success criteria**:
- No residual is paid without explicit operator approval
- Hold reason is recorded in audit log
- Summary provides clear picture of outgoing obligations

#### 3.3 — Revenue Dashboard
**Objective**: Platform-level financial overview.

**Display** (admin dashboard, new section on existing DashboardPage):
- Total platform volume (all merchants, this month / trailing 12 months)
- NMI residual received (from NMI reporting API)
- Total agency payouts (from residual ledger)
- Net retained revenue (NMI residual - agency payouts)
- Active merchants / active agencies / new this month
- Chargeback ratio across portfolio

**Implementation**:
- New admin endpoint: `GET /api/v1/admin/analytics/revenue`
- Aggregates from ResidualEntry + ResidualPayout tables
- NMI volume pulled from existing DailyMetrics or NMI Query API

**Success criteria**:
- Operator sees clear revenue waterfall
- Numbers reconcile: NMI residual = agency share + platform retained

#### 3.4 — Chargeback Monitoring & Alerts
**Objective**: Proactive chargeback ratio monitoring per merchant.

**Implementation**:
- Daily job: for each active merchant, calculate CB ratio = chargebacks / transactions (rolling 30 days)
- Thresholds: >0.5% → warning email to operator, >0.7% → critical alert, >1.0% → auto-flag for review
- Display CB ratio on merchant detail page with color coding
- Admin endpoint: `GET /api/v1/admin/chargebacks/risk` — merchants ranked by CB ratio

**Success criteria**:
- Operator notified before merchants hit card network thresholds
- CB ratio visible per merchant in admin
- High-risk merchants flagged automatically

---

### Work Stream 4: Payment Feature Depth

#### 4.1 — Subscription & Recurring Billing
**Objective**: Merchants can create subscription plans and enroll customers.

**Schema**:
```prisma
model SubscriptionPlan {
  id            String    @id @default(cuid())
  merchantId    String
  merchant      Merchant  @relation(fields: [merchantId], references: [id])
  name          String
  amount        Int       // cents
  currency      String    @default("usd")
  interval      BillingInterval
  trialDays     Int       @default(0)
  active        Boolean   @default(true)
  subscriptions Subscription[]
  createdAt     DateTime  @default(now())
  updatedAt     DateTime  @updatedAt
}

model Subscription {
  id                String    @id @default(cuid())
  merchantId        String
  merchant          Merchant  @relation(fields: [merchantId], references: [id])
  planId            String
  plan              SubscriptionPlan @relation(fields: [planId], references: [id])
  customerId        String
  customer          Customer  @relation(fields: [customerId], references: [id])
  nmiSubscriptionId String?   // if using NMI native recurring
  status            SubscriptionStatus @default(ACTIVE)
  currentPeriodStart DateTime
  currentPeriodEnd   DateTime
  trialEnd          DateTime?
  canceledAt        DateTime?
  cancelReason      String?
  failedAttempts    Int       @default(0)
  transactions      Transaction[]
  createdAt         DateTime  @default(now())
  updatedAt         DateTime  @updatedAt
}

enum BillingInterval {
  WEEKLY
  MONTHLY
  QUARTERLY
  ANNUAL
}

enum SubscriptionStatus {
  TRIALING
  ACTIVE
  PAST_DUE
  PAUSED
  CANCELED
}
```

**Endpoints (merchant-facing)**:
- `POST /api/v1/merchant/subscriptions/plans` — create plan
- `GET /api/v1/merchant/subscriptions/plans` — list plans
- `PATCH /api/v1/merchant/subscriptions/plans/:id` — update/deactivate plan
- `GET /api/v1/merchant/subscriptions` — list active subscriptions
- `POST /api/v1/merchant/subscriptions/:id/cancel` — cancel subscription

**Endpoints (checkout-facing)**:
- `POST /api/v1/checkout/subscribe` — customer enrolls: tokenize card → store in Customer Vault → create subscription → first charge

**Recurring charge execution**:
- Option A (preferred): Use NMI's native recurring billing API. On subscription create, call NMI to set up recurring plan. NMI fires charges automatically and sends webhooks.
- Option B (fallback): pg-boss scheduled job runs daily, finds subscriptions due for charge, executes via stored Customer Vault ID.

**Failed payment handling**:
- On NMI failed charge webhook → increment `failedAttempts`
- Retry schedule: day 1, 3, 7 after failure
- After 3 failures → status = PAST_DUE → dunning email to customer
- After 7 days past due with no successful retry → status = CANCELED

**Success criteria**:
- Merchant can create plans and view subscribers
- Customers can subscribe via checkout
- Recurring charges execute automatically
- Failed payments retry with dunning

#### 4.2 — Payment Links (Enhanced)
**Objective**: Merchants can generate shareable payment links with more options.

**Schema**:
```prisma
model PaymentLink {
  id            String    @id @default(cuid())
  merchantId    String
  merchant      Merchant  @relation(fields: [merchantId], references: [id])
  amount        Int?      // cents, null = customer enters amount
  currency      String    @default("usd")
  description   String
  slug          String    @unique  // short URL path
  singleUse     Boolean   @default(false)
  expiresAt     DateTime?
  active        Boolean   @default(true)
  timesUsed     Int       @default(0)
  metadata      Json?
  createdAt     DateTime  @default(now())
  updatedAt     DateTime  @updatedAt
}
```

**Endpoints**:
- `POST /api/v1/merchant/payment-links` — create link
- `GET /api/v1/merchant/payment-links` — list links with usage stats
- `PATCH /api/v1/merchant/payment-links/:id` — update/deactivate
- `GET /api/v1/pay/:slug` — public: renders checkout page for this link

**Success criteria**:
- Merchant generates links from dashboard
- Links support fixed or open amount
- Single-use links deactivate after payment
- Expiry enforced
- Usage tracking (opens, completions)

#### 4.3 — Invoice Builder
**Objective**: Merchants can create and send invoices with online payment.

**Schema**:
```prisma
model Invoice {
  id            String    @id @default(cuid())
  merchantId    String
  merchant      Merchant  @relation(fields: [merchantId], references: [id])
  customerId    String?
  customer      Customer? @relation(fields: [customerId], references: [id])
  invoiceNumber String    // merchant-scoped sequential (INV-0001)
  status        InvoiceStatus @default(DRAFT)
  subtotal      Int       // cents
  taxRate       Int       @default(0) // basis points (e.g., 825 = 8.25%)
  taxAmount     Int       @default(0)
  total         Int
  amountPaid    Int       @default(0)
  currency      String    @default("usd")
  dueDate       DateTime
  customerEmail String
  customerName  String
  notes         String?
  lineItems     InvoiceLineItem[]
  payments      Transaction[]
  sentAt        DateTime?
  paidAt        DateTime?
  createdAt     DateTime  @default(now())
  updatedAt     DateTime  @updatedAt
}

model InvoiceLineItem {
  id          String  @id @default(cuid())
  invoiceId   String
  invoice     Invoice @relation(fields: [invoiceId], references: [id])
  description String
  quantity    Int
  unitPrice   Int     // cents
  amount      Int     // cents (quantity * unitPrice)
  sortOrder   Int     @default(0)
}

enum InvoiceStatus {
  DRAFT
  SENT
  VIEWED
  PARTIALLY_PAID
  PAID
  OVERDUE
  CANCELED
}
```

**Endpoints**:
- `POST /api/v1/merchant/invoices` — create invoice with line items
- `GET /api/v1/merchant/invoices` — list with status filter
- `GET /api/v1/merchant/invoices/:id` — detail
- `PATCH /api/v1/merchant/invoices/:id` — update draft
- `POST /api/v1/merchant/invoices/:id/send` — email invoice to customer
- `GET /api/v1/invoice/:id` — public: renders invoice + payment form

**Partial payments**:
- Invoice tracks `amountPaid` vs `total`
- Multiple payments allowed against one invoice
- Status transitions: SENT → PARTIALLY_PAID → PAID

**Success criteria**:
- Merchant creates invoices with line items from dashboard
- Customer receives email with pay link
- Partial payments supported
- Invoice status updates on payment

#### 4.4 — Apple Pay & Google Pay
**Objective**: Digital wallet support in checkout.

**Current state**: CheckoutPage already has UI stubs for digital wallets. Backend schema supports GOOGLE_PAY and APPLE_PAY payment methods.

**Implementation**:
- **Apple Pay**: Host `/.well-known/apple-developer-merchantid-domain-association` on checkout domain. Implement Apple Pay JS on web. Pass `applepay_payment_data` to NMI.
- **Google Pay**: Configure Google Pay Business Console with gateway=nmi. Implement Google Pay API on web. Pass decrypted token to NMI.
- Both wallets surface through NMI's Collect.js Payment Component when configured — may just need enabling + styling.

**GHL context**: Apple/Google Pay in GHL requires either GHL custom payment provider support for wallets OR our checkout rendered in an iframe within GHL.

**Success criteria**:
- Apple Pay works on Safari/iOS
- Google Pay works on Chrome/Android
- Both route through NMI, appear in transaction history as correct payment method
- Works in both standalone checkout and GHL-embedded checkout

#### 4.5 — 3DS Authentication Enhancement
**Objective**: Configurable 3DS enforcement rules.

**Implementation**:
- Merchant settings: 3DS mode = OFF / SMART / ALWAYS
- SMART mode: require 3DS for transactions > $X, new customers, non-US cards
- Pass `Checkout Public Key` to Collect.js for 3DS flows
- Handle 3DS redirect and return within checkout page
- Store 3DS authentication result with transaction

**Success criteria**:
- Merchant can configure 3DS policy
- 3DS challenge renders in checkout
- Auth result passed to NMI for liability shift

---

### Work Stream 5: GHL Integration Depth

#### 5.1 — queryUrl Payment Processing
**Objective**: GHL sends payment requests to our queryUrl endpoint, we route to NMI.

**Current state**: `POST /api/v1/ghl/webhook` exists for install/uninstall. queryUrl endpoint needs to be built or verified.

**Implementation**:
- `POST /api/v1/ghl/query` — receives GHL payment context (amount, customer, order ID, payment token)
- Validates GHL HMAC signature
- Maps GHL sub-account → merchant → NMI MID
- Processes payment via NMI
- Returns success/failure in GHL expected format
- GHL updates order status based on response

**Success criteria**:
- GHL order form payment → our queryUrl → NMI → confirmation back to GHL
- Works for one-time payments
- Transaction recorded in our system with GHL order reference
- Merchant sees GHL-originated transactions in dashboard

#### 5.2 — GHL Marketplace Listing
**Objective**: Published app in GHL marketplace.

**Requirements**:
- App icon, screenshots, description
- OAuth scopes documented
- Webhook endpoints for install/uninstall (already built)
- SSO endpoint (already built)
- queryUrl registered as custom payment provider
- Test accounts for GHL review

**Success criteria**:
- App approved and listed in GHL marketplace
- Agencies can install with one click
- SSO into merchant dashboard from GHL

---

### Work Stream 6: Webhook & Notification System

#### 6.1 — Complete Webhook Delivery Worker
**Objective**: Actually deliver queued webhook events to merchant endpoints.

**Current state**: `WebhookEvent` and `MerchantWebhookDelivery` tables exist. Events are created in post-payment pipeline. But no worker POSTs to merchant URLs.

**Implementation**:
- pg-boss job handler: `webhook-delivery`
- On trigger: read `WebhookEvent`, look up merchant's `webhookUrl`
- POST JSON payload to merchant URL with:
  - `X-GHP-Signature` header (HMAC-SHA256)
  - `X-GHP-Timestamp` header
  - `X-GHP-Event-Type` header
- Record attempt in `MerchantWebhookDelivery`
- Retry on failure: 3 attempts with exponential backoff (10s, 60s, 300s)
- After 3 failures: mark event as `FAILED`, stop retrying

**Event types**:
- `payment.completed` — successful card/ACH payment
- `payment.failed` — declined transaction
- `payment.refunded` — full or partial refund
- `chargeback.created` — new dispute
- `chargeback.updated` — dispute status change
- `subscription.created` — new subscription
- `subscription.canceled` — subscription canceled
- `subscription.payment_failed` — recurring charge failed
- `invoice.paid` — invoice payment received

**Payload format** (standardized):
```json
{
  "id": "evt_xxx",
  "type": "payment.completed",
  "created": "2026-04-27T12:00:00Z",
  "data": {
    "transaction_id": "txn_xxx",
    "amount": 5000,
    "currency": "usd",
    "status": "captured",
    "payment_method": "card",
    "customer": { "id": "cus_xxx", "email": "..." }
  }
}
```

**Success criteria**:
- Webhook events delivered to merchant URLs
- Signed with HMAC
- Retries on failure
- Delivery status visible in merchant dashboard (already has WebhooksPage)

#### 6.2 — Email Notification System
**Objective**: Transactional emails for key events.

**Implementation**:
- Integrate SendGrid or Postmark (pick one)
- Email templates (stored as code, not in DB):
  - Merchant welcome (on onboarding complete)
  - Transaction receipt (on successful payment, to customer)
  - Chargeback alert (to merchant)
  - Subscription created/canceled confirmation (to customer)
  - Invoice sent (to customer)
  - Payment failed / dunning (to customer)
  - Residual statement (monthly, to agency)
  - Tier upgrade notification (to agency)
- Use `NotificationSchedule` table (already exists) for scheduled sends
- pg-boss job handler for async email delivery

**Success criteria**:
- Key lifecycle emails sent
- Emails are branded (white-label merchant name where applicable)
- Delivery tracked in notification schedule table

---

### Work Stream 7: NMI Integration Completion

#### 7.1 — NMI Partner Boarding API (Production)
**Objective**: Real merchant boarding via NMI's partner API (currently mock mode).

**Current state**: `nmi-boarding.service.ts` has `submitBoardingApplication()` and `checkBoardingStatus()` but falls back to mock when `NMI_PARTNER_ID` / `NMI_PARTNER_KEY` not set.

**Implementation**:
- Configure production NMI partner credentials
- Test real boarding flow in sandbox: submit application → NMI underwrites → returns MID + security key
- Handle async boarding (application may be PENDING for manual review)
- Background job to poll boarding status for pending applications
- On approval: store encrypted NMI credentials on merchant, update onboarding status, trigger welcome email

**Success criteria**:
- Real merchants boarded via API (not manual)
- Pending applications polled and resolved
- Merchant credentials encrypted at rest
- Onboarding flow end-to-end without manual intervention

#### 7.2 — NMI Reporting API Integration
**Objective**: Pull per-merchant volume and residual data from NMI.

**Implementation**:
- Use NMI Query API (`query.php`) to pull transaction summaries per merchant per period
- Daily job: sync settlement data for reconciliation
- Monthly job: pull volume data for residual calculation
- Store sync results in new `NmiSyncLog` table for audit

**Endpoints**:
- `GET /api/v1/admin/nmi/sync-status` — last sync time, any errors
- `POST /api/v1/admin/nmi/sync` — trigger manual sync

**Success criteria**:
- Daily volume data matches our transaction table (within tolerance)
- Monthly residual input data sourced from NMI, not our DB
- Sync failures alert operator

#### 7.3 — NMI Customer Vault Management
**Objective**: Proper lifecycle management for stored payment methods.

**Implementation**:
- On successful first transaction, ensure `customer_vault=add_customer` is passed
- Store `customer_vault_id` on Customer record (already in schema)
- Enable NMI Auto Card Updater (configuration, not code)
- Enable Network Tokenization in NMI portal
- Customer can update card via checkout session (new vault entry, old one deleted)
- Customer can delete stored payment method

**Success criteria**:
- Cards stored in NMI vault, not our DB
- Auto card updater reduces declines
- Customers can manage stored cards

---

### Work Stream 8: Observability & Production Readiness

#### 8.1 — Structured Logging
**Objective**: All logs are structured JSON with correlation IDs.

**Implementation**:
- Replace any `console.log` with structured logger (pino — already Fastify default)
- Every request gets a `requestId` (Fastify provides this)
- Every transaction flow gets a `correlationId` that follows through: checkout → backend → NMI call → webhook → post-payment pipeline
- Log fields: `timestamp`, `level`, `requestId`, `correlationId`, `merchantId`, `action`, `result`, `duration_ms`
- Sensitive fields (card data, keys) masked in all log output

**Success criteria**:
- All logs structured JSON
- Correlation ID traces full transaction lifecycle
- No sensitive data in logs

#### 8.2 — Health Checks & Monitoring
**Objective**: Comprehensive health endpoints and alerting.

**Implementation**:
- Expand `GET /health` to check: PostgreSQL, Redis, NMI API reachability
- Add `GET /health/ready` — readiness probe (all dependencies connected)
- Add `GET /health/live` — liveness probe (process alive)
- Integrate Sentry for error tracking (backend + all frontends)
- Key alerts:
  - NMI API call failure rate > 1% over 5 minutes
  - Webhook delivery failure rate > 5%
  - Database connection pool exhaustion
  - Redis connection failure
  - Any 5xx response rate spike

**Success criteria**:
- Health endpoints return dependency status
- Errors captured in Sentry with full context
- Critical alerts fire within 1 minute

#### 8.3 — Database Migrations & Backup Strategy
**Objective**: Safe schema migrations and data backup.

**Implementation**:
- All schema changes via Prisma migrations (already in place)
- Pre-migration backup: `pg_dump` before every deploy that includes migrations
- Fly.io PostgreSQL: enable WAL archiving for point-in-time recovery
- Test migration rollback procedure
- Document: how to restore from backup, how to rollback a migration

**Success criteria**:
- Every deploy with migrations has a backup
- PITR enabled
- Rollback procedure documented and tested

#### 8.4 — CI/CD Pipeline
**Objective**: Automated build, test, and deploy.

**Implementation** (GitHub Actions):
- On PR: lint, typecheck, unit tests, build all packages
- On merge to main: deploy to staging (Fly.io dev)
- On tag/release: deploy to production (Fly.io prod)
- Migration safety: run `prisma migrate diff` to detect destructive changes, require manual approval

**Success criteria**:
- No manual deploys
- PRs blocked on failing checks
- Staging auto-deploys on merge
- Production deploys require explicit trigger

#### 8.5 — Testing Foundation
**Objective**: Baseline test coverage for critical paths.

**Implementation**:
- **Unit tests** (Vitest): NMI service (mock HTTP), encryption utils, residual calculation logic, webhook signature generation/verification
- **Integration tests** (Vitest + Prisma test DB): checkout flow (create session → process payment → webhook → post-payment pipeline), merchant onboarding flow, agency registration + attribution, residual calculation job
- **E2E tests** (Playwright): checkout page card payment, merchant dashboard login + view transactions, agency dashboard login + view portfolio
- Test database: separate PostgreSQL database, reset between test suites

**Success criteria**:
- Critical payment flow covered by integration tests
- Residual calculation covered by unit tests
- CI runs tests on every PR
- No mocked NMI calls in integration tests (use NMI sandbox)

#### 8.6 — Secrets Management
**Objective**: No secrets in code, env files, or logs.

**Implementation**:
- All secrets via environment variables injected by Fly.io secrets
- `ENCRYPTION_KEY`, `NMI_PARTNER_KEY`, `DATABASE_URL`, `REDIS_URL`, `JWT_SECRET`, `SENDGRID_API_KEY`
- Document rotation procedure for each secret
- `.env.example` has placeholder values only (already exists)
- Pre-commit hook: scan for potential secret leaks (use `gitleaks` or similar)

**Success criteria**:
- No secrets in git history
- Rotation procedure documented per secret
- Pre-commit scan blocks accidental leaks

#### 8.7 — PCI Compliance (SAQ-A)
**Objective**: Document and maintain PCI DSS compliance.

**Our scope**: SAQ-A (card data never touches our servers — Collect.js / Payment Component handles all card input).

**Requirements**:
- Verify no card data in any log, database field, or API response
- All payment pages served over HTTPS
- CSP headers prevent card data exfiltration (already partially implemented)
- Collect.js loaded from NMI CDN only (no self-hosted copy)
- Annual SAQ-A self-assessment questionnaire completed
- Vulnerability scan (ASV) quarterly

**Success criteria**:
- SAQ-A self-assessment completed
- No PAN, CVV, or expiry anywhere in our stack
- CSP headers locked down on checkout pages

---

### Work Stream 9: Legal & Compliance

#### 9.1 — Terms of Service & Agreements
**Objective**: Legal documents for all user types.

**Documents needed**:
- Platform Terms of Service (merchants agree on signup)
- Agency Partner Agreement (agencies agree on registration)
- Privacy Policy (CCPA/GDPR compliant)
- Acceptable Use Policy

**Implementation**:
- Attorney drafts documents
- Acceptance recorded in database with timestamp and version
- Checkbox on registration flows
- Version tracking: if ToS updated, re-acceptance required on next login

#### 9.2 — Tax Compliance
**Objective**: Proper tax documentation for agency payouts.

**Implementation**:
- W-9 collection from agencies before first payout (upload form in agency settings)
- Track `taxIdOnFile` boolean on Agency record
- Annual 1099-NEC generation for agencies paid > $600/year
- Integration with tax filing service (e.g., Tax1099.com API) or manual export

**Success criteria**:
- No payout without W-9 on file
- 1099s generated annually
- Records retained per IRS requirements

---

## Technology Stack

| Layer | Choice | Notes |
|-------|--------|-------|
| Backend | Fastify + TypeScript | Already built |
| Database | PostgreSQL 16 | Already built, Prisma ORM |
| Cache/Queue | Redis 7 + pg-boss | Already built |
| Auth | JWT (15m access + 7d refresh) | Already built |
| Email | SendGrid or Postmark | To be integrated |
| Frontend | React 18 + Vite + TailwindCSS | Already built |
| SDK | Vanilla TypeScript | Already built |
| Hosting | Fly.io | Already configured |
| Monitoring | Sentry | To be integrated |
| CI/CD | GitHub Actions | To be built |
| Secrets | Fly.io secrets + env vars | Partially in place |

## ACH Provider Note

The codebase currently uses **Seamlesschex** for ACH payments, with per-merchant API keys. The original spec references NMI/Paya for ACH. Current implementation stays on Seamlesschex unless there's a compelling reason to migrate. Both providers work; Seamlesschex is already integrated and tested.

---

## Critical Rules

1. **Never store raw card numbers, CVVs, or expiry dates.** Use Collect.js + Customer Vault ID only.
2. **Never log card data.** Mask all payment fields before logging.
3. **All NMI API calls server-side.** Never expose Security Key to browser.
4. **Idempotency keys on all payment requests.** Format: `{merchantId}:{sessionId}:{timestamp}`. Store in Redis with 24h TTL. Reject duplicates.
5. **Webhook signatures on all outbound webhooks.** HMAC-SHA256 with per-merchant secret.
6. **Attribution is immutable.** Once a merchant is attributed to an agency, never change it.
7. **Residual ledger is append-only.** Never update finalized entries. Create adjustment entries.
8. **All financial data has audit trail.** `created_at`, `updated_at`, immutable audit log. No soft deletes on financial records.
9. **Encryption at rest** for all credentials and PII stored in our database.
10. **We are an ISO referral partner.** We never hold, transmit, or settle merchant funds. NMI is the merchant of record.
