/**
 * NoStripeTax SDK — Entry Point
 *
 * Provides a Stripe Elements-style API for embedding PCI-compliant card fields.
 *
 * Script tag usage:
 *   <script src="https://js.nostripetax.com/v1.js"></script>
 *   <script>
 *     const nst = NoStripeTax('tok_live_abc123');
 *   </script>
 *
 * NPM usage:
 *   import { NoStripeTax } from '@nostripetax/js';
 *   const nst = NoStripeTax('tok_live_abc123');
 */

import { NoStripeTaxSDK } from "./nostripetax.js";
import type { NoStripeTaxOptions } from "./types.js";

// Re-export all types for consumers
export type {
  ElementType,
  CardBrand,
  ElementChangeEvent,
  ElementEventType,
  ElementOptions,
  ElementStyle,
  ElementStyleVariant,
  ElementsOptions,
  ElementsTheme,
  TokenResult,
  TokenSuccess,
  TokenError,
  NoStripeTaxOptions,
} from "./types.js";

export { NoStripeTaxSDK } from "./nostripetax.js";
export { Elements } from "./elements.js";
export { Element } from "./element.js";

/**
 * Create a new NoStripeTax SDK instance.
 *
 * This is the primary entry point — both for UMD (window.NoStripeTax)
 * and ESM (import { NoStripeTax }) usage.
 *
 * @param tokenizationKey - Merchant's NMI tokenization key
 * @param options - Optional SDK configuration
 * @returns NoStripeTaxSDK instance
 *
 * @example
 * ```js
 * const nst = NoStripeTax('tok_live_abc123');
 * const elements = nst.elements();
 * const cardNumber = elements.create('cardNumber');
 * cardNumber.mount('#card-number');
 * ```
 */
export function NoStripeTax(
  tokenizationKey: string,
  options?: NoStripeTaxOptions
): NoStripeTaxSDK {
  return new NoStripeTaxSDK(tokenizationKey, options);
}

// Attach to window for UMD/script tag usage
if (typeof window !== "undefined") {
  (window as any).NoStripeTax = NoStripeTax;
}
