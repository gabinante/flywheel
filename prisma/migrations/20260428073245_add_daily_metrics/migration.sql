-- CreateTable
CREATE TABLE "DailyMetrics" (
    "id" TEXT NOT NULL,
    "date" DATE NOT NULL,
    "totalVolume" INTEGER NOT NULL,
    "totalTransactions" INTEGER NOT NULL DEFAULT 0,
    "totalChargebacks" INTEGER NOT NULL DEFAULT 0,
    "activeMerchants" INTEGER NOT NULL DEFAULT 0,
    "activeAgencies" INTEGER NOT NULL DEFAULT 0,
    "newMerchants" INTEGER NOT NULL DEFAULT 0,
    "newAgencies" INTEGER NOT NULL DEFAULT 0,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updatedAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "DailyMetrics_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "DailyMetrics_date_key" ON "DailyMetrics"("date");

-- CreateIndex
CREATE INDEX "DailyMetrics_date_idx" ON "DailyMetrics"("date");
