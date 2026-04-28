/**
 * NoStripeTax SDK — Unit Tests
 *
 * Tests the SDK initialization, Elements factory, Element lifecycle,
 * event system, and tokenization flow.
 */

import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { NoStripeTax, NoStripeTaxSDK } from "../index.js";
import { Elements } from "../elements.js";
import { Element } from "../element.js";
import { resetCollectLoader } from "../collect-loader.js";

// ─── Helpers ─────────────────────────────────────────────────────────

function createContainer(id: string): HTMLElement {
  const el = document.createElement("div");
  el.id = id;
  document.body.appendChild(el);
  return el;
}

function cleanupContainers(): void {
  document.body.innerHTML = "";
}

// ─── SDK Initialization ──────────────────────────────────────────────

describe("NoStripeTax()", () => {
  it("returns a NoStripeTaxSDK instance with a valid key", () => {
    const nst = NoStripeTax("tok_test_key");
    expect(nst).toBeInstanceOf(NoStripeTaxSDK);
  });

  it("throws if tokenization key is empty", () => {
    expect(() => NoStripeTax("")).toThrow("requires a tokenization key");
  });

  it("throws if tokenization key is not a string", () => {
    // @ts-expect-error — testing runtime guard
    expect(() => NoStripeTax(123)).toThrow("requires a tokenization key");
  });

  it("throws if tokenization key is undefined", () => {
    // @ts-expect-error — testing runtime guard
    expect(() => NoStripeTax(undefined)).toThrow(
      "requires a tokenization key"
    );
  });

  it("attaches NoStripeTax to window for UMD usage", () => {
    expect(typeof (window as any).NoStripeTax).toBe("function");
  });

  it("window.NoStripeTax creates an SDK instance", () => {
    const nst = (window as any).NoStripeTax("tok_test_key");
    expect(nst).toBeInstanceOf(NoStripeTaxSDK);
  });
});

// ─── Elements Factory ────────────────────────────────────────────────

describe("nst.elements()", () => {
  let nst: NoStripeTaxSDK;

  beforeEach(() => {
    nst = NoStripeTax("tok_test_key");
  });

  it("returns an Elements instance", () => {
    const elements = nst.elements();
    expect(elements).toBeInstanceOf(Elements);
  });

  it("accepts optional theme configuration", () => {
    const elements = nst.elements({
      theme: { primaryColor: "#4F46E5", borderRadius: "8px" },
    });
    expect(elements).toBeInstanceOf(Elements);
  });
});

// ─── Element Creation ────────────────────────────────────────────────

describe("elements.create()", () => {
  let elements: Elements;

  beforeEach(() => {
    const nst = NoStripeTax("tok_test_key");
    elements = nst.elements();
  });

  it("creates a cardNumber element", () => {
    const el = elements.create("cardNumber");
    expect(el).toBeInstanceOf(Element);
    expect(el.type).toBe("cardNumber");
  });

  it("creates a cardExpiry element", () => {
    const el = elements.create("cardExpiry");
    expect(el).toBeInstanceOf(Element);
    expect(el.type).toBe("cardExpiry");
  });

  it("creates a cardCvv element", () => {
    const el = elements.create("cardCvv");
    expect(el).toBeInstanceOf(Element);
    expect(el.type).toBe("cardCvv");
  });

  it("creates a cardComplete element", () => {
    const el = elements.create("cardComplete");
    expect(el).toBeInstanceOf(Element);
    expect(el.type).toBe("cardComplete");
  });

  it("accepts placeholder option", () => {
    const el = elements.create("cardNumber", {
      placeholder: "4242 4242 4242 4242",
    });
    expect(el.options.placeholder).toBe("4242 4242 4242 4242");
  });

  it("accepts style options", () => {
    const el = elements.create("cardNumber", {
      style: { base: { fontSize: "16px", color: "#333" } },
    });
    expect(el.options.style?.base?.fontSize).toBe("16px");
  });

  it("throws on invalid element type", () => {
    // @ts-expect-error — testing runtime guard
    expect(() => elements.create("invalid")).toThrow("Invalid element type");
  });

  it("throws on duplicate element type", () => {
    elements.create("cardNumber");
    expect(() => elements.create("cardNumber")).toThrow("already created");
  });

  it("getElements returns all created elements", () => {
    elements.create("cardNumber");
    elements.create("cardExpiry");
    elements.create("cardCvv");
    expect(elements.getElements()).toHaveLength(3);
  });
});

// ─── Element Mount ───────────────────────────────────────────────────

describe("element.mount()", () => {
  let elements: Elements;

  beforeEach(() => {
    cleanupContainers();
    resetCollectLoader();
    const nst = NoStripeTax("tok_test_key");
    elements = nst.elements();
  });

  afterEach(() => {
    cleanupContainers();
  });

  it("mounts to a CSS selector", () => {
    createContainer("card-number");
    const el = elements.create("cardNumber");
    el.mount("#card-number");
    expect(el.isMounted).toBe(true);
  });

  it("mounts to an HTMLElement", () => {
    const container = createContainer("card-number");
    const el = elements.create("cardNumber");
    el.mount(container);
    expect(el.isMounted).toBe(true);
  });

  it("creates a Collect.js-targeted inner div", () => {
    const container = createContainer("card-number");
    const el = elements.create("cardNumber");
    el.mount("#card-number");
    const innerDiv = container.querySelector("#nst-ccnumber");
    expect(innerDiv).not.toBeNull();
  });

  it("throws if selector not found", () => {
    const el = elements.create("cardNumber");
    expect(() => el.mount("#nonexistent")).toThrow("Mount target not found");
  });

  it("throws if already mounted", () => {
    createContainer("card-number");
    const el = elements.create("cardNumber");
    el.mount("#card-number");
    expect(() => el.mount("#card-number")).toThrow("already mounted");
  });

  it("creates correct inner div ids for each type", () => {
    createContainer("cn");
    createContainer("ce");
    createContainer("cv");

    const cn = elements.create("cardNumber");
    const ce = elements.create("cardExpiry");
    const cv = elements.create("cardCvv");

    cn.mount("#cn");
    ce.mount("#ce");
    cv.mount("#cv");

    expect(document.querySelector("#nst-ccnumber")).not.toBeNull();
    expect(document.querySelector("#nst-ccexp")).not.toBeNull();
    expect(document.querySelector("#nst-cvv")).not.toBeNull();
  });
});

// ─── Element Unmount / Destroy ───────────────────────────────────────

describe("element.unmount() / destroy()", () => {
  beforeEach(() => {
    cleanupContainers();
    resetCollectLoader();
  });

  afterEach(() => {
    cleanupContainers();
  });

  it("unmounts from the DOM", () => {
    createContainer("card-number");
    const nst = NoStripeTax("tok_test_key");
    const elements = nst.elements();
    const el = elements.create("cardNumber");
    el.mount("#card-number");
    expect(el.isMounted).toBe(true);

    el.unmount();
    expect(el.isMounted).toBe(false);
    expect(document.querySelector("#nst-ccnumber")).toBeNull();
  });

  it("destroy removes listeners and unmounts", () => {
    createContainer("card-number");
    const nst = NoStripeTax("tok_test_key");
    const elements = nst.elements();
    const el = elements.create("cardNumber");
    el.mount("#card-number");

    const callback = vi.fn();
    el.on("change", callback);
    el.destroy();

    expect(el.isMounted).toBe(false);
    // After destroy, emitting should not call the listener
    el._emit("change", { complete: true, error: null, brand: null, empty: false });
    expect(callback).not.toHaveBeenCalled();
  });
});

// ─── Element Events ──────────────────────────────────────────────────

describe("element.on() / off()", () => {
  it("registers and fires change event", () => {
    const nst = NoStripeTax("tok_test_key");
    const elements = nst.elements();
    const el = elements.create("cardNumber");

    const callback = vi.fn();
    el.on("change", callback);

    const event = {
      complete: true,
      error: null,
      brand: "visa" as const,
      empty: false,
    };
    el._emit("change", event);

    expect(callback).toHaveBeenCalledOnce();
    expect(callback).toHaveBeenCalledWith(event);
  });

  it("registers and fires focus event", () => {
    const nst = NoStripeTax("tok_test_key");
    const elements = nst.elements();
    const el = elements.create("cardNumber");

    const callback = vi.fn();
    el.on("focus", callback);
    el._emit("focus");

    expect(callback).toHaveBeenCalledOnce();
  });

  it("registers and fires blur event", () => {
    const nst = NoStripeTax("tok_test_key");
    const elements = nst.elements();
    const el = elements.create("cardNumber");

    const callback = vi.fn();
    el.on("blur", callback);
    el._emit("blur");

    expect(callback).toHaveBeenCalledOnce();
  });

  it("registers and fires ready event", () => {
    const nst = NoStripeTax("tok_test_key");
    const elements = nst.elements();
    const el = elements.create("cardNumber");

    const callback = vi.fn();
    el.on("ready", callback);
    el._emit("ready");

    expect(callback).toHaveBeenCalledOnce();
  });

  it("removes a listener with off()", () => {
    const nst = NoStripeTax("tok_test_key");
    const elements = nst.elements();
    const el = elements.create("cardNumber");

    const callback = vi.fn();
    el.on("change", callback);
    el.off("change", callback);
    el._emit("change", {
      complete: true,
      error: null,
      brand: null,
      empty: false,
    });

    expect(callback).not.toHaveBeenCalled();
  });

  it("supports multiple listeners for the same event", () => {
    const nst = NoStripeTax("tok_test_key");
    const elements = nst.elements();
    const el = elements.create("cardNumber");

    const cb1 = vi.fn();
    const cb2 = vi.fn();
    el.on("change", cb1);
    el.on("change", cb2);
    el._emit("change", {
      complete: false,
      error: null,
      brand: null,
      empty: true,
    });

    expect(cb1).toHaveBeenCalledOnce();
    expect(cb2).toHaveBeenCalledOnce();
  });

  it("handles listener errors gracefully", () => {
    const nst = NoStripeTax("tok_test_key");
    const elements = nst.elements();
    const el = elements.create("cardNumber");

    const consoleErrorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const badCallback = vi.fn(() => {
      throw new Error("listener error");
    });
    const goodCallback = vi.fn();

    el.on("change", badCallback);
    el.on("change", goodCallback);
    el._emit("change", {
      complete: true,
      error: null,
      brand: null,
      empty: false,
    });

    // Bad callback threw, but good callback still ran
    expect(badCallback).toHaveBeenCalledOnce();
    expect(goodCallback).toHaveBeenCalledOnce();
    expect(consoleErrorSpy).toHaveBeenCalled();

    consoleErrorSpy.mockRestore();
  });
});

// ─── createToken ─────────────────────────────────────────────────────

describe("nst.createToken()", () => {
  let nst: NoStripeTaxSDK;

  beforeEach(() => {
    cleanupContainers();
    resetCollectLoader();
    nst = NoStripeTax("tok_test_key");
  });

  afterEach(() => {
    cleanupContainers();
    delete (window as any).CollectJS;
  });

  it("returns error if CollectJS is not loaded", async () => {
    const elements = nst.elements();
    const result = await nst.createToken(elements);
    expect(result.error).toBeDefined();
    expect(result.error!.message).toContain("Collect.js not loaded");
  });

  it("resolves with token on successful tokenization", async () => {
    // Mock CollectJS
    (window as any).CollectJS = {
      configure: vi.fn((config: any) => {
        // Store the callback for later
        (window as any)._collectJsCallback = config.callback;
      }),
      startPaymentRequest: vi.fn(() => {
        // Simulate async callback
        setTimeout(() => {
          (window as any)._collectJsCallback({ token: "tok_abc123" });
        }, 10);
      }),
    };

    const elements = nst.elements();
    const result = await nst.createToken(elements);
    expect(result.token).toBe("tok_abc123");
    expect(result.error).toBeUndefined();
  });

  it("resolves with error on validation failure", async () => {
    (window as any).CollectJS = {
      configure: vi.fn((config: any) => {
        (window as any)._collectJsValidationCallback =
          config.validationCallback;
      }),
      startPaymentRequest: vi.fn(() => {
        setTimeout(() => {
          (window as any)._collectJsValidationCallback(
            "ccnumber",
            false,
            "Invalid card number"
          );
        }, 10);
      }),
    };

    const elements = nst.elements();
    const result = await nst.createToken(elements);
    expect(result.error).toBeDefined();
    expect(result.error!.message).toContain("Invalid card number");
  });

  it("resolves with error on timeout", async () => {
    (window as any).CollectJS = {
      configure: vi.fn((config: any) => {
        (window as any)._collectJsTimeoutCallback = config.timeoutCallback;
      }),
      startPaymentRequest: vi.fn(() => {
        setTimeout(() => {
          (window as any)._collectJsTimeoutCallback();
        }, 10);
      }),
    };

    const elements = nst.elements();
    const result = await nst.createToken(elements);
    expect(result.error).toBeDefined();
    expect(result.error!.message).toContain("timed out");
  });

  it("resolves with error if startPaymentRequest throws", async () => {
    (window as any).CollectJS = {
      configure: vi.fn(),
      startPaymentRequest: vi.fn(() => {
        throw new Error("Collect.js internal error");
      }),
    };

    const elements = nst.elements();
    const result = await nst.createToken(elements);
    expect(result.error).toBeDefined();
    expect(result.error!.message).toContain("Collect.js internal error");
  });
});

// ─── Legacy mount() ──────────────────────────────────────────────────

describe("nst.mount() (legacy iframe mode)", () => {
  beforeEach(() => {
    cleanupContainers();
  });

  afterEach(() => {
    cleanupContainers();
  });

  it("creates an iframe in the target container", () => {
    createContainer("checkout");
    const nst = NoStripeTax("tok_test_key");
    nst.mount("#checkout", "session_123");

    const iframe = document.querySelector("#checkout iframe") as HTMLIFrameElement;
    expect(iframe).not.toBeNull();
    expect(iframe.src).toContain("session/session_123");
    expect(iframe.src).toContain("embed=true");
  });

  it("uses custom baseUrl when provided", () => {
    createContainer("checkout");
    const nst = NoStripeTax("tok_test_key");
    nst.mount("#checkout", "session_456", {
      baseUrl: "https://custom.example.com",
    });

    const iframe = document.querySelector("#checkout iframe") as HTMLIFrameElement;
    expect(iframe.src).toContain("https://custom.example.com/session/session_456");
  });

  it("throws if selector not found", () => {
    const nst = NoStripeTax("tok_test_key");
    expect(() => nst.mount("#nonexistent", "session_123")).toThrow(
      "Mount target not found"
    );
  });
});

// ─── Legacy redirect() ──────────────────────────────────────────────

describe("nst.redirect() (legacy redirect mode)", () => {
  it("redirects to checkout URL", () => {
    // Mock window.location.href
    const originalHref = window.location.href;
    const hrefSetter = vi.fn();
    Object.defineProperty(window, "location", {
      value: { ...window.location, href: originalHref },
      writable: true,
    });
    Object.defineProperty(window.location, "href", {
      set: hrefSetter,
      get: () => originalHref,
    });

    const nst = NoStripeTax("tok_test_key");
    nst.redirect("session_789");

    expect(hrefSetter).toHaveBeenCalledWith(
      "https://checkout.nostripetax.com/session/session_789"
    );
  });
});
