import { useState, useEffect, useCallback } from 'react';

/**
 * Detects if the merchant dashboard is running inside a GHL iframe
 * and provides utilities for communicating with the parent frame.
 */
export function useGhlEmbed() {
  const [isEmbedded, setIsEmbedded] = useState(false);

  useEffect(() => {
    // Detect iframe embedding
    const embedded = window.self !== window.top;
    setIsEmbedded(embedded);

    if (embedded) {
      // Store embed state so Layout can read it synchronously on mount
      sessionStorage.setItem('ghl_embedded', 'true');
    }
  }, []);

  // Send height updates to GHL parent so it can resize the iframe
  const notifyResize = useCallback(() => {
    if (!isEmbedded) return;
    try {
      const height = document.documentElement.scrollHeight;
      window.parent.postMessage(
        { type: 'ghp:resize', data: { height } },
        '*' // GHL parent origin varies by region
      );
    } catch {
      // Cross-origin - expected if parent doesn't listen
    }
  }, [isEmbedded]);

  // Observe DOM changes and notify parent of height changes
  useEffect(() => {
    if (!isEmbedded) return;

    // Initial resize
    notifyResize();

    const observer = new ResizeObserver(() => notifyResize());
    observer.observe(document.body);

    // Also listen for route changes (SPA navigation)
    const interval = setInterval(notifyResize, 500);

    return () => {
      observer.disconnect();
      clearInterval(interval);
    };
  }, [isEmbedded, notifyResize]);

  // Navigate to a URL in the parent frame (break out of iframe)
  const navigateParent = useCallback((url: string) => {
    if (isEmbedded) {
      window.parent.postMessage({ type: 'ghp:navigate', data: { url } }, '*');
    } else {
      window.location.href = url;
    }
  }, [isEmbedded]);

  return { isEmbedded, notifyResize, navigateParent };
}

/**
 * Synchronous check for iframe state (works before React hydration).
 * Used by Layout to avoid sidebar flash.
 */
export function isGhlEmbedded(): boolean {
  try {
    return window.self !== window.top || sessionStorage.getItem('ghl_embedded') === 'true';
  } catch {
    // Cross-origin access throws - means we're in an iframe
    return true;
  }
}
