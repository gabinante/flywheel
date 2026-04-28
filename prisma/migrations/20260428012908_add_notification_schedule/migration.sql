-- CreateEnum
CREATE TYPE "NotificationStatus" AS ENUM ('QUEUED', 'SENT', 'DELIVERED', 'FAILED', 'BOUNCED');

-- CreateTable
CREATE TABLE "NotificationSchedule" (
    "id" TEXT NOT NULL,
    "merchantId" TEXT,
    "agencyId" TEXT,
    "templateId" TEXT NOT NULL,
    "to" TEXT NOT NULL,
    "variables" JSONB NOT NULL,
    "status" "NotificationStatus" NOT NULL DEFAULT 'QUEUED',
    "attempts" INTEGER NOT NULL DEFAULT 0,
    "lastError" TEXT,
    "sentAt" TIMESTAMP(3),
    "deliveredAt" TIMESTAMP(3),
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "NotificationSchedule_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE INDEX "NotificationSchedule_merchantId_idx" ON "NotificationSchedule"("merchantId");

-- CreateIndex
CREATE INDEX "NotificationSchedule_agencyId_idx" ON "NotificationSchedule"("agencyId");

-- CreateIndex
CREATE INDEX "NotificationSchedule_status_idx" ON "NotificationSchedule"("status");

-- CreateIndex
CREATE INDEX "NotificationSchedule_templateId_idx" ON "NotificationSchedule"("templateId");

-- CreateIndex
CREATE INDEX "NotificationSchedule_createdAt_idx" ON "NotificationSchedule"("createdAt");
