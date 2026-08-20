/**
 * Only the shapes this app touches — the admin write endpoints, plus the
 * handful of reference reads their forms need (analysts, markets). Not a
 * mirror of the public frontend's types.ts; extend as new admin screens need
 * more of the API surface.
 */

export type PackageCode = 'ordinary' | 'vip' | 'akatambula';
export const PACKAGE_CODES: PackageCode[] = ['ordinary', 'vip', 'akatambula'];

export type MarketCode =
  | 'ONE_X_TWO'
  | 'DOUBLE_CHANCE'
  | 'BTTS'
  | 'OVER_UNDER_1_5'
  | 'OVER_UNDER_2_5'
  | 'OVER_UNDER_3_5';

export type UserRole = 'user' | 'analyst' | 'admin';

export interface SessionUser {
  id: string;
  email: string;
  name: string;
  phone?: string;
  role: UserRole;
  createdAt: string;
}

export interface Analyst {
  id: string;
  slug: string;
  name: string;
  handle: string;
  initials: string;
  bio: string;
  packages: PackageCode[];
  joinedAt: string;
}

export interface Outcome {
  value: string;
  label: string;
  shortLabel: string;
}

export interface MarketType {
  code: MarketCode;
  displayName: string;
  shortName: string;
  tabLabel: string;
  slug: string;
  outcomes: Outcome[];
}

export type SlipStatus = 'draft' | 'open' | 'settled' | 'void';

export interface Slip {
  id: string;
  packageCode: PackageCode;
  title: string;
  analystIds: string[];
  publishedAt: string | null;
  priceUgx: number;
  status: SlipStatus;
  tipCount: number;
  totalOdds: number;
  wonTips?: number;
  settledOdds?: number;
}

export interface Tip {
  id: string;
  slipId: string;
  analystId: string;
  matchId: string | null;
  fixtureLabel: string;
  marketLabel: string;
  selectionLabel: string;
  marketType: MarketCode | null;
  selectionValue: string | null;
  odds: number;
  kickoffAt: string;
  note?: string;
}

export interface TipResult {
  tipId: string;
  wasCorrect: boolean;
  actualOutcome: string;
  settledAt: string;
  settledBy: 'auto' | 'admin';
}

export interface SlipWithTips extends Slip {
  tips: Tip[];
  results?: TipResult[];
  analysts: Analyst[];
  unlocked: boolean;
}

export interface IngestProviderStatus {
  provider: string;
  requestsUsed: number;
  dailyLimit: number;
  throttledAt?: string;
  lastFetchedAt?: string;
  leagues: number;
  ungradedPastKickoff: number;
}

export interface AdminIngestStatus {
  providers: IngestProviderStatus[];
  openCorrectionReviews: number;
}

export interface RevenueBucket {
  key: string;
  label: string;
  purchases: number;
  grossUgx: number;
}

export interface RevenueReport {
  from: string;
  to: string;
  grossUgx: number;
  refundedUgx: number;
  netUgx: number;
  paidPurchases: number;
  refundedPurchases: number;
  pendingPurchases: number;
  pendingUgx: number;
  failedPurchases: number;
  byPackage: RevenueBucket[];
  byAnalyst: RevenueBucket[];
  bySlip: RevenueBucket[];
  byDay: RevenueBucket[];
  byMobileProvider: RevenueBucket[];
}

export interface PaymentLedgerRow {
  traceCode: string;
  providerTxnId?: string;
  status: string;
  amountUgx: number;
  phoneNumber: string;
  mobileProvider?: string;
  packageCode: string;
  slipTitle: string;
  userEmail: string;
  createdAt: string;
  settledAt?: string;
}

export interface PaymentTrace {
  traceCode: string;
  reference: string;
  providerUuid?: string;
  providerTxnId?: string;
  status: string;
  amountUgx: number;
  phoneNumber: string;
  mobileProvider?: string;
  createdAt: string;
  settledAt?: string;
  purchaseId: string;
  purchaseStatus: string;
  paidAt?: string;
  slipId: string;
  slipTitle: string;
  packageCode: PackageCode;
  userId: string;
  userEmail: string;
  userName: string;
}
