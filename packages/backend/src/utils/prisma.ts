/**
 * Prisma client singleton with encryption middleware.
 *
 * All database access should use this module's exported `prisma` instance
 * to ensure sensitive fields are transparently encrypted/decrypted.
 */

import { PrismaClient } from '@prisma/client';
import { encryptionMiddleware } from '../middleware/encryption.middleware.js';
import { validateEncryptionKey } from './encryption.js';

// Validate encryption key on import — fail fast if misconfigured
validateEncryptionKey();

export const prisma = new PrismaClient({
  log: process.env.NODE_ENV === 'development' ? ['query', 'error', 'warn'] : ['error'],
});

// Register encryption middleware for transparent field-level encryption.
// Note: Prisma.Middleware and our local Middleware type are structurally
// compatible — both accept (params, next) => Promise<any>.
prisma.$use(encryptionMiddleware as any);
