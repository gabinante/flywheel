import { z } from 'zod';

const envSchema = z.object({
  NODE_ENV: z.enum(['development', 'production', 'test']).default('development'),
  PORT: z.string().default('3000'),
  HOST: z.string().default('0.0.0.0'),

  DATABASE_URL: z.string(),
  REDIS_URL: z.string().default('redis://localhost:6379'),

  API_BASE_URL: z.string().default('http://localhost:3000'),
  FRONTEND_URL: z.string().default('http://localhost:5173'),
  ADMIN_URL: z.string().default('http://localhost:5174'),
  MERCHANT_URL: z.string().default('http://localhost:5175'),
  CHECKOUT_URL: z.string().default('http://localhost:5176'),

  JWT_SECRET: z.string(),
  JWT_REFRESH_SECRET: z.string(),
  JWT_EXPIRES_IN: z.string().default('15m'),
  JWT_REFRESH_EXPIRES_IN: z.string().default('7d'),

  // NMI platform-level (webhook verification + mock mode)
  NMI_WEBHOOK_SECRET: z.string().default(''),
  NMI_MOCK_MODE: z.string().default('true'),

  // NMI Boarding API (ISO partner credentials)
  NMI_BOARDING_API_URL: z.string().default('https://secure.nmi.com/api/boarding/'),
  NMI_PARTNER_ID: z.string().default(''),
  NMI_PARTNER_KEY: z.string().default(''),

  GOOGLE_PAY_MERCHANT_ID: z.string().default(''),
  APPLE_PAY_MERCHANT_ID: z.string().default(''),

  // Seamlesschex platform-level (webhook verification + sandbox mode)
  SEAMLESSCHEX_WEBHOOK_SECRET: z.string().default(''),
  SEAMLESSCHEX_SANDBOX: z.string().default('true'),

  GHL_CLIENT_ID: z.string().default(''),
  GHL_CLIENT_SECRET: z.string().default(''),
  GHL_REDIRECT_URI: z.string().default('http://localhost:3000/api/v1/ghl/callback'),
  GHL_APP_ID: z.string().default(''),
  GHL_SSO_KEY: z.string().default(''),

  ENCRYPTION_KEY: z.string().default('change-me-32-byte-hex-string-here'),
});

export const env = envSchema.parse(process.env);
