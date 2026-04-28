import { FastifyInstance } from 'fastify';
import crypto from 'crypto';

/**
 * JWT claims for agency tokens.
 */
export interface AgencyJwtPayload {
  sub: string;       // agencyId
  type: 'agency';
  iat?: number;
  exp?: number;
}

/**
 * Refresh token payload stored alongside the JWT.
 */
export interface RefreshTokenPayload {
  sub: string;
  type: 'agency';
  tokenType: 'refresh';
}

/**
 * Sign an access JWT (15 min) for an agency.
 */
export function signAccessToken(app: FastifyInstance, agencyId: string): string {
  return app.jwt.sign(
    { sub: agencyId, type: 'agency' },
    { expiresIn: '15m' }
  );
}

/**
 * Sign a refresh token (7 days) for an agency.
 */
export function signRefreshToken(app: FastifyInstance, agencyId: string): string {
  return app.jwt.sign(
    { sub: agencyId, type: 'agency', tokenType: 'refresh' },
    { expiresIn: '7d' }
  );
}

/**
 * Generate a token pair (access + refresh) for an agency.
 */
export function generateTokenPair(app: FastifyInstance, agencyId: string) {
  return {
    accessToken: signAccessToken(app, agencyId),
    refreshToken: signRefreshToken(app, agencyId),
    expiresIn: 900, // 15 minutes in seconds
  };
}

/**
 * Generate a cryptographically random token for refresh purposes (alternative approach).
 */
export function generateRandomToken(): string {
  return crypto.randomBytes(32).toString('hex');
}
