/**
 * NoStripeTax — Main SDK class
 *
 * Provides a Stripe Elements-style API for embedding PCI-compliant card fields.
 * Wraps NMI's Collect.js inline variant.
 *
 * Usage:
 *   const nst = NoStripeTax('tok_live_abc123');
 *   const elements = nst.elements();
 *   const cardNumber = elements.create('cardNumber');
 *   cardNumber.mount('#card-number');
 *   const { token, error } = await nst.createToken(elements);
 */

import { Elements } from "./elements.js";
import type {
  ElementsOptions,
  TokenResult,
  NoStripeTaxOptions,
} from "./types.js";

export class NoStripeTaxSDK {
  private _tokenizationKey: string;
  private _options: NoStripeTaxOptions;

  constructor(tokenizationKey: string, options: NoStripeTaxOptions = {}) {
    if (!tokenizationKey || typeof tokenizationKey !== "string") {
      throw new Error(
        "NoStripeTax requires a tokenization key. Usage: NoStripeTax('tok_xxx')"
      );
    }
    this._tokenizationKey = tokenizationKey;
    this._options = options;
  }

  /**
   * Create an Elements factory for building individual card field elements.
   *
   * @param options - Optional theme and configuration
   * @returns Elements factory instance
   */
  elements(options: ElementsOptions = {}): Elements {
    return new Elements(
      this._tokenizationKey,
      options,
      this._options.collectJsUrl
    );
  }

  /**
   * Tokenize the card data collected by the Elements.
   *
   * Calls Collect.js startPaymentRequest() and wraps the callback-based API
   * in a Promise. Card data never touches the merchant's page or our servers.
   *
   * @param elements - The Elements instance containing mounted card fields
   * @returns Promise resolving to { token } on success or { error } on failure
   */
  createToken(elements: Elements): Promise<TokenResult> {
    return new Promise<TokenResult>((resolve, reject) => {
      // Ensure Collect.js is available
      if (!window.CollectJS) {
        resolve({
          error: {
            message:
              "Collect.js not loaded. Ensure all elements are mounted before calling createToken().",
          },
        });
        return;
      }

      let resolved = false;

      const safeResolve = (result: TokenResult) => {
        if (!resolved) {
          resolved = true;
          resolve(result);
        }
      };

      // Reconfigure Collect.js with our callback to capture the token
      try {
        window.CollectJS.configure({
          callback: (response) => {
            if (response && response.token) {
              safeResolve({ token: response.token });
            } else {
              safeResolve({
                error: { message: "Tokenization failed: no token returned" },
              });
            }
          },
          validationCallback: (
            field: string,
            valid: boolean,
            message: string
          ) => {
            if (!valid) {
              safeResolve({
                error: { message: `${field}: ${message}` },
              });
            }
          },
          timeoutCallback: () => {
            safeResolve({
              error: {
                message: "Tokenization timed out. Please try again.",
              },
            });
          },
          timeoutDuration: 30000,
        });

        window.CollectJS.startPaymentRequest();
      } catch (err) {
        const message = err instanceof Error ? err.message : String(err);
        safeResolve({ error: { message: `Tokenization error: ${message}` } });
      }
    });
  }

  /**
   * Mount the full checkout iframe (legacy mode).
   * This preserves backward compatibility with the existing SDK API.
   *
   * @param selector - CSS selector for the container element
   * @param sessionId - Checkout session ID
   * @param options - Optional configuration
   */
  mount(
    selector: string,
    sessionId: string,
    options: { baseUrl?: string } = {}
  ): void {
    const container = document.querySelector<HTMLElement>(selector);
    if (!container) {
      throw new Error(`Mount target not found: ${selector}`);
    }

    const baseUrl =
      options.baseUrl ?? "https://checkout.nostripetax.com";
    const iframe = document.createElement("iframe");
    iframe.src = `${baseUrl}/session/${sessionId}?embed=true`;
    iframe.style.width = "100%";
    iframe.style.minHeight = "600px";
    iframe.style.border = "none";
    iframe.setAttribute("frameborder", "0");
    iframe.setAttribute("allowpaymentrequest", "true");
    iframe.setAttribute(
      "allow",
      "payment *; publickey-credentials-get *"
    );

    container.innerHTML = "";
    container.appendChild(iframe);
  }

  /**
   * Redirect to the hosted checkout page (legacy mode).
   *
   * @param sessionId - Checkout session ID
   * @param options - Optional configuration
   */
  redirect(
    sessionId: string,
    options: { baseUrl?: string } = {}
  ): void {
    const baseUrl =
      options.baseUrl ?? "https://checkout.nostripetax.com";
    window.location.href = `${baseUrl}/session/${sessionId}`;
  }
}
