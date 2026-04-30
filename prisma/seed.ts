import { PrismaClient } from '@prisma/client';
import bcrypt from 'bcryptjs';
import { createHash, randomBytes } from 'crypto';

const prisma = new PrismaClient();

function generateApiKey(prefix: string): string {
  return `${prefix}_${randomBytes(24).toString('base64url')}`;
}

function hashApiKey(key: string): string {
  return createHash('sha256').update(key).digest('hex');
}

async function main() {
  console.log('Seeding database...');

  // Create super admin
  const adminPassword = await bcrypt.hash('admin123456', 12);
  const admin = await prisma.adminUser.upsert({
    where: { email: 'admin@shamroq.com' },
    update: {},
    create: {
      email: 'admin@shamroq.com',
      passwordHash: adminPassword,
      name: 'Super Admin',
      role: 'SUPER_ADMIN',
    },
  });
  console.log('Admin created:', admin.email);

  // Create test merchant (with their own NMI credentials placeholder)
  const merchantPassword = await bcrypt.hash('merchant123456', 12);
  const liveKey = generateApiKey('sk_live');
  const testKey = generateApiKey('sk_test');

  const merchant = await prisma.merchant.upsert({
    where: { contactEmail: 'test@merchant.com' },
    update: {},
    create: {
      businessName: 'Test Merchant',
      contactEmail: 'test@merchant.com',
      passwordHash: merchantPassword,
      status: 'ACTIVE',
      // Merchant's own NMI credentials (placeholder — replace with real keys)
      nmiSecurityKey: 'merchant_nmi_security_key_placeholder',
      nmiTokenizationKey: 'merchant_nmi_tokenization_key_placeholder',
      apiKeyLive: liveKey,
      apiKeyTest: testKey,
      apiKeyLiveHash: hashApiKey(liveKey),
      apiKeyTestHash: hashApiKey(testKey),
      webhookUrl: null,
    },
  });
  console.log('Merchant created:', merchant.contactEmail);
  console.log('  Live API key:', liveKey);
  console.log('  Test API key:', testKey);

  // Create test agency with referral code
  const agencyPassword = await bcrypt.hash('agency123456', 12);
  const testAgency = await prisma.agency.upsert({
    where: { referralCode: 'agency-test-001' },
    update: {},
    create: {
      name: 'Test Agency',
      contactEmail: 'test@agency.example.com',
      contactPhone: '+15551234567',
      passwordHash: agencyPassword,
      referralCode: 'agency-test-001',
      tier: 'TIER_1',
      status: 'ACTIVE',
      payoutEmail: 'payouts@agency.example.com',
      taxIdOnFile: false,
    },
  });
  console.log('Agency created:', {
    id: testAgency.id,
    name: testAgency.name,
    referralCode: testAgency.referralCode,
    tier: testAgency.tier,
    status: testAgency.status,
  });

  // Create a second agency referred by the first (two-tier referral)
  const referredAgencyPassword = await bcrypt.hash('referred123456', 12);
  const referredAgency = await prisma.agency.upsert({
    where: { referralCode: 'agency-referred-002' },
    update: {},
    create: {
      name: 'Referred Agency',
      contactEmail: 'referred@agency.example.com',
      passwordHash: referredAgencyPassword,
      referralCode: 'agency-referred-002',
      referredByAgencyId: testAgency.id,
      tier: 'TIER_1',
      status: 'ACTIVE',
      taxIdOnFile: false,
    },
  });
  console.log('Referred agency created:', {
    id: referredAgency.id,
    name: referredAgency.name,
    referralCode: referredAgency.referralCode,
    referredByAgencyId: referredAgency.referredByAgencyId,
  });

  console.log('Seed complete!');
}

main()
  .catch(console.error)
  .finally(() => prisma.$disconnect());
