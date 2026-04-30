-- CreateTable
CREATE TABLE "GhlOAuthState" (
    "id" TEXT NOT NULL,
    "state" TEXT NOT NULL,
    "referralCode" TEXT,
    "createdAt" TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "expiresAt" TIMESTAMP(3) NOT NULL,

    CONSTRAINT "GhlOAuthState_pkey" PRIMARY KEY ("id")
);

-- CreateIndex
CREATE UNIQUE INDEX "GhlOAuthState_state_key" ON "GhlOAuthState"("state");

-- CreateIndex
CREATE INDEX "GhlOAuthState_state_idx" ON "GhlOAuthState"("state");

-- CreateIndex
CREATE INDEX "GhlOAuthState_expiresAt_idx" ON "GhlOAuthState"("expiresAt");
