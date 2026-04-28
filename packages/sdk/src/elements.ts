/**
 * Elements — Factory for creating and managing individual Element instances
 *
 * When all created elements are mounted, this class triggers Collect.js loading
 * and configuration for inline mode.
 */

import { Element } from "./element.js";
import { loadCollectJs } from "./collect-loader.js";
import type {
  ElementType,
  ElementOptions,
  ElementsOptions,
  ElementChangeEvent,
  CollectJSFieldConfig,
  ELEMENT_TYPE_TO_COLLECTJS,
} from "./types.js";

const ELEMENT_TYPE_MAP: Record<string, string> = {
  cardNumber: "ccnumber",
  cardExpiry: "ccexp",
  cardCvv: "cvv",
};

export class Elements {
  private _tokenizationKey: string;
  private _options: ElementsOptions;
  private _collectJsUrl?: string;
  private _elements: Element[] = [];
  private _collectJsReady = false;
  private _configurePromise: Promise<void> | null = null;

  constructor(
    tokenizationKey: string,
    options: ElementsOptions = {},
    collectJsUrl?: string
  ) {
    this._tokenizationKey = tokenizationKey;
    this._options = options;
    this._collectJsUrl = collectJsUrl;
  }

  /**
   * Create a new Element instance.
   * The element is lazy — Collect.js is not loaded until mount() is called.
   */
  create(type: ElementType, options: ElementOptions = {}): Element {
    // Validate type
    const validTypes: ElementType[] = [
      "cardNumber",
      "cardExpiry",
      "cardCvv",
      "cardComplete",
      "achAccount",
    ];
    if (!validTypes.includes(type)) {
      throw new Error(
        `Invalid element type "${type}". Valid types: ${validTypes.join(", ")}`
      );
    }

    // Check for duplicate types
    if (this._elements.some((el) => el.type === type)) {
      throw new Error(
        `Element of type "${type}" already created. Each type can only be created once per Elements instance.`
      );
    }

    const element = new Element(type, options);
    this._elements.push(element);

    // Wrap mount to detect when all elements are mounted
    const originalMount = element.mount.bind(element);
    element.mount = (selectorOrElement: string | HTMLElement) => {
      originalMount(selectorOrElement);
      this._checkAllMounted();
    };

    return element;
  }

  /**
   * Get all created elements.
   */
  getElements(): readonly Element[] {
    return this._elements;
  }

  /**
   * Check if Collect.js has been loaded and configured.
   */
  get isReady(): boolean {
    return this._collectJsReady;
  }

  /**
   * Wait for Collect.js to be ready. Resolves once all elements are mounted
   * and Collect.js is configured.
   */
  async waitForReady(): Promise<void> {
    if (this._collectJsReady) return;
    if (this._configurePromise) return this._configurePromise;

    // If not all mounted yet, return a promise that resolves when they are
    return new Promise((resolve) => {
      const interval = setInterval(() => {
        if (this._collectJsReady) {
          clearInterval(interval);
          resolve();
        }
      }, 50);
    });
  }

  // ─── Internal ──────────────────────────────────────────────────────

  /**
   * Check if all created elements are mounted, and if so, load Collect.js.
   */
  private _checkAllMounted(): void {
    const allMounted = this._elements.every((el) => el.isMounted);
    if (!allMounted || this._configurePromise) return;

    this._configurePromise = this._loadAndConfigure();
  }

  /**
   * Load the Collect.js script and configure it for all mounted elements.
   */
  private async _loadAndConfigure(): Promise<void> {
    try {
      const collectJS = await loadCollectJs(
        this._tokenizationKey,
        this._collectJsUrl
      );

      // Build fields config for Collect.js inline mode
      const fields: Record<string, CollectJSFieldConfig> = {};

      for (const element of this._elements) {
        const collectJsField = ELEMENT_TYPE_MAP[element.type];
        if (!collectJsField) continue;

        fields[collectJsField] = {
          selector: `#${element._getCollectJsId()}`,
          title: this._getFieldTitle(element.type),
          placeholder: element.options.placeholder ?? "",
        };
      }

      // Configure Collect.js
      collectJS.configure({
        variant: "inline",
        fields,
        callback: () => {
          // Default no-op; overridden by createToken
        },
        fieldsAvailableCallback: () => {
          this._collectJsReady = true;
          for (const element of this._elements) {
            element._ready = true;
            element._emit("ready");
          }
        },
        validationCallback: (
          field: string,
          valid: boolean,
          message: string
        ) => {
          // Map Collect.js field name back to our element
          const element = this._findElementByCollectJsField(field);
          if (element) {
            element._emit("change", {
              complete: valid,
              error: valid ? null : { message },
              brand: null,
              empty: false,
            } satisfies ElementChangeEvent);
          }
        },
      } as any);
    } catch (err) {
      console.error("[NoStripeTax] Failed to load Collect.js:", err);
      throw err;
    }
  }

  /**
   * Find an element by its Collect.js field name.
   */
  private _findElementByCollectJsField(field: string): Element | undefined {
    for (const element of this._elements) {
      if (ELEMENT_TYPE_MAP[element.type] === field) {
        return element;
      }
    }
    return undefined;
  }

  /**
   * Get a human-readable title for a field type.
   */
  private _getFieldTitle(type: ElementType): string {
    const titles: Record<string, string> = {
      cardNumber: "Card Number",
      cardExpiry: "Expiration Date",
      cardCvv: "CVV",
      cardComplete: "Payment Information",
    };
    return titles[type] ?? type;
  }
}
