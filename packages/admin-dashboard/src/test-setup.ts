import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

// Polyfill ResizeObserver for jsdom (used by Recharts ResponsiveContainer)
globalThis.ResizeObserver = class ResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
} as unknown as typeof ResizeObserver;

// Ensure cleanup after each test (needed with globals: false)
afterEach(() => {
  cleanup();
});
