/**
 * NoStripeTax SDK — TypeScript interfaces
 */

// ─── Element Types ───────────────────────────────────────────────────

/** Supported element types that can be created via elements.create() */
export type ElementType =
  | "cardNumber"
  | "cardExpiry"
  | "cardCvv"
  | "cardComplete"
  | "achAccount";

/** Maps our element types to Collect.js field selectors */
export const ELEMENT_TYPE_TO_COLLECTJS: Record<string, string> = {
  cardNumber: "ccnumber",
  cardExpiry: "ccexp",
  cardCvv: "cvv",
};

/** Card brand identifiers */
export type CardBrand =
  | "visa"
  | "mastercard"
  | "amex"
  | "discover"
  | "diners"
  | "jcb"
  | null;

// ─── Element Events ──────────────────────────────────────────────────

export interface ElementChangeEvent {
  /** Whether the field value is complete and valid */
  complete: boolean;
  /** Validation error, if any */
  error: { message: string } | null;
  /** Detected card brand (only for cardNumber) */
  brand: CardBrand;
  /** Whether the field is empty */
  empty: boolean;
}

export type ElementEventType = "change" | "focus" | "blur" | "ready";

export type ElementEventCallback<T = void> = T extends "change"
  ? (event: ElementChangeEvent) => void
  : () => void;

// ─── Element Options ─────────────────────────────────────────────────

export interface ElementStyleVariant {
  fontSize?: string;
  color?: string;
  fontFamily?: string;
  fontWeight?: string;
  lineHeight?: string;
  letterSpacing?: string;
  textDecoration?: string;
  textTransform?: string;
  "::placeholder"?: { color?: string };
}

export interface ElementStyle {
  base?: ElementStyleVariant;
  focus?: ElementStyleVariant;
  error?: ElementStyleVariant;
  empty?: ElementStyleVariant;
}

export interface ElementOptions {
  placeholder?: string;
  style?: ElementStyle;
}

// ─── Elements Factory Options ────────────────────────────────────────

export interface ElementsTheme {
  primaryColor?: string;
  borderRadius?: string;
  fontFamily?: string;
  fontSize?: string;
}

export interface ElementsOptions {
  theme?: ElementsTheme;
}

// ─── Token Response ──────────────────────────────────────────────────

export interface TokenSuccess {
  token: string;
  error?: undefined;
}

export interface TokenError {
  token?: undefined;
  error: { message: string };
}

export type TokenResult = TokenSuccess | TokenError;

// ─── Collect.js Global Types ─────────────────────────────────────────

export interface CollectJSResponse {
  token: string;
  card?: {
    number: string;
    bin: string;
    exp: string;
    type: string;
  };
}

export interface CollectJSConfig {
  paymentSelector?: string;
  variant?: string;
  callback: (response: CollectJSResponse) => void;
  validationCallback?: (
    field: string,
    valid: boolean,
    message: string
  ) => void;
  fieldsAvailableCallback?: () => void;
  timeoutCallback?: () => void;
  timeoutDuration?: number;
  fields?: Record<string, CollectJSFieldConfig>;
}

export interface CollectJSFieldConfig {
  selector?: string;
  title?: string;
  placeholder?: string;
}

export interface CollectJSGlobal {
  configure: (config: CollectJSConfig) => void;
  startPaymentRequest: () => void;
}

declare global {
  interface Window {
    CollectJS?: CollectJSGlobal;
    NoStripeTax?: typeof import("./index.js").NoStripeTax;
  }
}

// ─── SDK Options ─────────────────────────────────────────────────────

export interface NoStripeTaxOptions {
  /** Override the Collect.js script URL (for testing) */
  collectJsUrl?: string;
}
