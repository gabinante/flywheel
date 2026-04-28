import { createHash, randomBytes } from 'crypto';

export function generateApiKey(prefix: 'pk_live' | 'pk_test' | 'sk_live' | 'sk_test'): string {
  const key = randomBytes(24).toString('base64url');
  return `${prefix}_${key}`;
}

export function hashApiKey(key: string): string {
  return createHash('sha256').update(key).digest('hex');
}
