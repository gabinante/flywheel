/**
 * Element — Individual PCI-compliant input field
 *
 * Each Element wraps a Collect.js inline iframe field (card number, expiry, CVV).
 * Elements are created via the Elements factory and mounted to DOM containers.
 */

import type {
  ElementType,
  ElementOptions,
  ElementEventType,
  ElementChangeEvent,
  CardBrand,
  ELEMENT_TYPE_TO_COLLECTJS,
} from "./types.js";

export class Element {
  readonly type: ElementType;
  readonly options: ElementOptions;

  private _mounted = false;
  private _container: HTMLElement | null = null;
  private _listeners: Map<ElementEventType, Set<(event?: any) => void>> =
    new Map();

  /** Set internally by Elements factory after Collect.js is configured */
  _ready = false;

  constructor(type: ElementType, options: ElementOptions = {}) {
    this.type = type;
    this.options = options;
  }

  /**
   * Mount this element to a DOM container.
   * @param selectorOrElement - CSS selector string or HTMLElement
   */
  mount(selectorOrElement: string | HTMLElement): void {
    if (this._mounted) {
      throw new Error(
        `Element "${this.type}" is already mounted. Call unmount() first.`
      );
    }

    const container =
      typeof selectorOrElement === "string"
        ? document.querySelector<HTMLElement>(selectorOrElement)
        : selectorOrElement;

    if (!container) {
      throw new Error(
        `Mount target not found: ${selectorOrElement}. Ensure the element exists in the DOM.`
      );
    }

    // Create the inner div that Collect.js will target
    const innerDiv = document.createElement("div");
    innerDiv.id = this._getCollectJsId();
    innerDiv.style.minHeight = "40px";
    container.appendChild(innerDiv);

    this._container = container;
    this._mounted = true;
  }

  /**
   * Register an event listener.
   */
  on(event: "change", callback: (event: ElementChangeEvent) => void): void;
  on(event: "focus" | "blur" | "ready", callback: () => void): void;
  on(event: ElementEventType, callback: (event?: any) => void): void {
    if (!this._listeners.has(event)) {
      this._listeners.set(event, new Set());
    }
    this._listeners.get(event)!.add(callback);
  }

  /**
   * Remove an event listener.
   */
  off(event: ElementEventType, callback: (event?: any) => void): void {
    this._listeners.get(event)?.delete(callback);
  }

  /**
   * Unmount this element from the DOM.
   */
  unmount(): void {
    if (this._container) {
      const innerDiv = this._container.querySelector(
        `#${this._getCollectJsId()}`
      );
      if (innerDiv) {
        innerDiv.remove();
      }
    }
    this._mounted = false;
    this._container = null;
    this._ready = false;
  }

  /**
   * Destroy this element completely, removing all listeners.
   */
  destroy(): void {
    this.unmount();
    this._listeners.clear();
  }

  /**
   * Check if this element is currently mounted.
   */
  get isMounted(): boolean {
    return this._mounted;
  }

  // ─── Internal Helpers ────────────────────────────────────────────

  /**
   * Get the DOM id that Collect.js will use to find this field.
   * Collect.js looks for elements with specific ids based on field type.
   */
  _getCollectJsId(): string {
    const idMap: Record<string, string> = {
      cardNumber: "nst-ccnumber",
      cardExpiry: "nst-ccexp",
      cardCvv: "nst-cvv",
      cardComplete: "nst-payment",
    };
    return idMap[this.type] ?? `nst-${this.type}`;
  }

  /**
   * Emit an event to all registered listeners.
   * Called by the Elements factory when Collect.js fires callbacks.
   */
  _emit(event: ElementEventType, data?: any): void {
    const listeners = this._listeners.get(event);
    if (listeners) {
      for (const cb of listeners) {
        try {
          cb(data);
        } catch (err) {
          console.error(
            `[NoStripeTax] Error in "${event}" listener for ${this.type}:`,
            err
          );
        }
      }
    }
  }
}
