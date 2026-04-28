import { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import fp from 'fastify-plugin';

/**
 * Decoded JWT payload shape for agency tokens.
 */
interface AgencyTokenPayload {
  sub: string;
  type: string;
  tokenType?: string;
  iat: number;
  exp: number;
}

/**
 * Extends FastifyRequest with the authenticated agency ID.
 */
declare module 'fastify' {
  interface FastifyRequest {
    agencyId?: string;
  }
}

/**
 * Agency auth plugin — verifies JWT and ensures the token type is 'agency'.
 * Rejects merchant/admin tokens with 401.
 *
 * Usage: Register on routes that require agency authentication.
 * After verification, request.agencyId is set.
 */
async function agencyAuthPlugin(app: FastifyInstance) {
  app.decorate('authenticateAgency', async function (
    request: FastifyRequest,
    reply: FastifyReply
  ) {
    try {
      // Verify JWT signature and expiry
      const decoded = await request.jwtVerify<AgencyTokenPayload>();

      // Ensure this is an agency token (not merchant/admin)
      if (decoded.type !== 'agency') {
        return reply.status(401).send({
          error: 'Unauthorized',
          message: 'Invalid token type: expected agency token',
        });
      }

      // Reject refresh tokens used as access tokens
      if (decoded.tokenType === 'refresh') {
        return reply.status(401).send({
          error: 'Unauthorized',
          message: 'Refresh tokens cannot be used for authentication',
        });
      }

      // Set agencyId on request for downstream handlers
      request.agencyId = decoded.sub;
    } catch (err) {
      return reply.status(401).send({
        error: 'Unauthorized',
        message: 'Invalid or expired token',
      });
    }
  });
}

export default fp(agencyAuthPlugin, {
  name: 'agency-auth',
  dependencies: ['@fastify/jwt'],
});

/**
 * Declare the authenticateAgency decorator on FastifyInstance.
 */
declare module 'fastify' {
  interface FastifyInstance {
    authenticateAgency: (
      request: FastifyRequest,
      reply: FastifyReply
    ) => Promise<void>;
  }
}
