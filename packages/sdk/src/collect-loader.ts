/**
 * Collect.js Script Loader
 *
 * Manages loading the NMI Collect.js script and configuring it for inline mode.
 * Ensures the script is loaded only once per page.
 */

import type { CollectJSGlobal } from "./types.js";

const DEFAULT_COLLECT_JS_URL = "https://secure.nmi.com/token/Collect.js";

let loadPromise: Promise<CollectJSGlobal> | null = null;
let loadedUrl: string | null = null;

/**
 * Load the Collect.js script with the given tokenization key.
 * Returns a promise that resolves when CollectJS is available on window.
 *
 * If already loaded with the same URL + key, returns the cached promise.
 */
export function loadCollectJs(
  tokenizationKey: string,
  scriptUrl?: string
): Promise<CollectJSGlobal> {
  const url = scriptUrl ?? DEFAULT_COLLECT_JS_URL;

  // Re-use existing load if URL matches
  if (loadPromise && loadedUrl === url) {
    return loadPromise;
  }

  loadedUrl = url;
  loadPromise = new Promise<CollectJSGlobal>((resolve, reject) => {
    // If CollectJS is already on the page, resolve immediately
    if (window.CollectJS) {
      resolve(window.CollectJS);
      return;
    }

    const script = document.createElement("script");
    script.src = url;
    script.setAttribute("data-tokenization-key", tokenizationKey);
    script.setAttribute("data-variant", "inline");
    script.async = true;

    script.onload = () => {
      // Collect.js may take a tick to attach to window
      const check = (attempts: number) => {
        if (window.CollectJS) {
          resolve(window.CollectJS);
        } else if (attempts < 50) {
          setTimeout(() => check(attempts + 1), 50);
        } else {
          reject(new Error("Collect.js loaded but CollectJS global not found"));
        }
      };
      check(0);
    };

    script.onerror = () => {
      loadPromise = null;
      loadedUrl = null;
      reject(new Error("Failed to load Collect.js script"));
    };

    document.head.appendChild(script);
  });

  return loadPromise;
}

/**
 * Reset the loader state (for testing).
 */
export function resetCollectLoader(): void {
  loadPromise = null;
  loadedUrl = null;
}
