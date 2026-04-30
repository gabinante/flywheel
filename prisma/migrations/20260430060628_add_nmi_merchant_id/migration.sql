-- Add nmi_merchant_id column to merchants table
-- Stores the NMI-assigned merchant ID (MID) after boarding approval

ALTER TABLE "merchants" ADD COLUMN "nmi_merchant_id" TEXT;
