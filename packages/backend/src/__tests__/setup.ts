/**
 * Jest global setup file.
 * Sets the minimum environment variables required for module-level evaluation
 * in test files (e.g., Prisma client init, Zod env parsing).
 *
 * Individual tests can override these values in beforeEach/afterEach.
 */

// Required for prisma.ts validateEncryptionKey() and env.ts parsing
process.env.ENCRYPTION_KEY = 'a'.repeat(64);
process.env.DATABASE_URL = 'postgresql://localhost:5432/shamroq_test';
process.env.JWT_SECRET = 'test-jwt-secret-for-testing-only-not-real';
process.env.JWT_REFRESH_SECRET = 'test-jwt-refresh-secret-for-testing-only';
