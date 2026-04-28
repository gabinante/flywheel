-- CreateEnum
CREATE TYPE "AgencyTier" AS ENUM ('TIER_1', 'TIER_2', 'TIER_3');

-- CreateEnum
CREATE TYPE "AgencyStatus" AS ENUM ('ACTIVE', 'SUSPENDED', 'CHURNED');

-- CreateEnum
CREATE TYPE "MerchantStatus" AS ENUM ('ACTIVE', 'INACTIVE', 'SUSPENDED');

-- CreateEnum
CREATE TYPE "ResidualStatus" AS ENUM ('PENDING', 'APPROVED', 'PAID', 'HELD');

-- CreateEnum
CREATE TYPE "PayoutStatus" AS ENUM ('PENDING', 'APPROVED', 'PAID', 'FAILED');

-- CreateTable
CREATE TABLE "Agency" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "contactEmail" TEXT NOT NULL,
    "contactPhone" TEXT,
    "passwordHash" TEXT NOT NULL,
    "referralCode" TEXT NOT NULL,
    "referredByAgencyId" TEXT,
    "tier" "AgencyTier" NOT NULL DEFAULT 'TIER_1',
    "tierOverride" BOOLEAN NOT NULL DEFAULT false,
    "status" "AgencyStatus" NOT NULL DEFAULT 'ACTIVE',
    "payoutEmail" TEXT,
    "payoutBankLast4" TEXT,
    "taxIdOnFile" BOOLEAN NOT NULL DEFAULT false,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "Agency_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "Merchant" (
    "id" TEXT NOT NULL,
    "name" TEXT NOT NULL,
    "email" TEXT NOT NULL,
    "phone" TEXT,
    "nmiMerchantId" TEXT,
    "ghlLocationId" TEXT,
    "status" "MerchantStatus" NOT NULL DEFAULT 'ACTIVE',
    "agencyId" TEXT,
    "attributedAt" TIMESTAMP(3),
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "Merchant_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "ResidualEntry" (
    "id" TEXT NOT NULL,
    "agencyId" TEXT NOT NULL,
    "merchantId" TEXT NOT NULL,
    "periodStart" TIMESTAMP(3) NOT NULL,
    "periodEnd" TIMESTAMP(3) NOT NULL,
    "merchantVolume" INTEGER NOT NULL,
    "nmiResidualEarned" INTEGER NOT NULL,
    "agencyBps" INTEGER NOT NULL,
    "agencyShare" INTEGER NOT NULL,
    "twoTierAgencyId" TEXT,
    "twoTierShare" INTEGER NOT NULL DEFAULT 0,
    "status" "ResidualStatus" NOT NULL DEFAULT 'PENDING',
    "approvedAt" TIMESTAMP(3),
    "approvedBy" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "ResidualEntry_pkey" PRIMARY KEY ("id")
);

-- CreateTable
CREATE TABLE "ResidualPayout" (
    "id" TEXT NOT NULL,
    "agencyId" TEXT NOT NULL,
    "periodStart" TIMESTAMP(3) NOT NULL,
    "totalAmount" INTEGER NOT NULL,
    "directShare" INTEGER NOT NULL,
    "twoTierShare" INTEGER NOT NULL,
    "method" TEXT NOT NULL,
    "reference" TEXT,
    "status" "PayoutStatus" NOT NULL DEFAULT 'PENDING',
    "paidAt" TIMESTAMP(3),
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "ResidualPayout_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "Agency_contactEmail_key" ON "Agency"("contactEmail");

-- CreateIndex
CREATE UNIQUE INDEX "Agency_referralCode_key" ON "Agency"("referralCode");

-- CreateIndex
CREATE UNIQUE INDEX "Merchant_email_key" ON "Merchant"("email");

-- CreateIndex
CREATE UNIQUE INDEX "Merchant_nmiMerchantId_key" ON "Merchant"("nmiMerchantId");

-- CreateIndex
CREATE UNIQUE INDEX "Merchant_ghlLocationId_key" ON "Merchant"("ghlLocationId");

-- CreateIndex
CREATE UNIQUE INDEX "ResidualEntry_agencyId_merchantId_periodStart_key" ON "ResidualEntry"("agencyId", "merchantId", "periodStart");

-- AddForeignKey
ALTER TABLE "Agency" ADD CONSTRAINT "Agency_referredByAgencyId_fkey" FOREIGN KEY ("referredByAgencyId") REFERENCES "Agency"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "Merchant" ADD CONSTRAINT "Merchant_agencyId_fkey" FOREIGN KEY ("agencyId") REFERENCES "Agency"("id") ON DELETE SET NULL ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "ResidualEntry" ADD CONSTRAINT "ResidualEntry_agencyId_fkey" FOREIGN KEY ("agencyId") REFERENCES "Agency"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "ResidualEntry" ADD CONSTRAINT "ResidualEntry_merchantId_fkey" FOREIGN KEY ("merchantId") REFERENCES "Merchant"("id") ON DELETE RESTRICT ON UPDATE CASCADE;

-- AddForeignKey
ALTER TABLE "ResidualPayout" ADD CONSTRAINT "ResidualPayout_agencyId_fkey" FOREIGN KEY ("agencyId") REFERENCES "Agency"("id") ON DELETE RESTRICT ON UPDATE CASCADE;
