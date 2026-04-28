import { PrismaClient } from "@prisma/client";

const prisma = new PrismaClient();

async function main() {
  // Upsert a test agency so the seed is idempotent
  const testAgency = await prisma.agency.upsert({
    where: { referralCode: "agency-test-001" },
    update: {},
    create: {
      name: "Test Agency",
      contactEmail: "test@agency.example.com",
      contactPhone: "+15551234567",
      passwordHash: "$2b$10$placeholder-hash-for-seeding-only",
      referralCode: "agency-test-001",
      tier: "TIER_1",
      status: "ACTIVE",
      payoutEmail: "payouts@agency.example.com",
      taxIdOnFile: false,
    },
  });

  console.log("Seeded test agency:", {
    id: testAgency.id,
    name: testAgency.name,
    referralCode: testAgency.referralCode,
    tier: testAgency.tier,
    status: testAgency.status,
  });

  // Seed a second agency referred by the first (two-tier referral)
  const referredAgency = await prisma.agency.upsert({
    where: { referralCode: "agency-referred-002" },
    update: {},
    create: {
      name: "Referred Agency",
      contactEmail: "referred@agency.example.com",
      passwordHash: "$2b$10$placeholder-hash-for-seeding-only",
      referralCode: "agency-referred-002",
      referredByAgencyId: testAgency.id,
      tier: "TIER_1",
      status: "ACTIVE",
      taxIdOnFile: false,
    },
  });

  console.log("Seeded referred agency:", {
    id: referredAgency.id,
    name: referredAgency.name,
    referralCode: referredAgency.referralCode,
    referredByAgencyId: referredAgency.referredByAgencyId,
  });
}

main()
  .then(async () => {
    await prisma.$disconnect();
  })
  .catch(async (e) => {
    console.error(e);
    await prisma.$disconnect();
    process.exit(1);
  });
