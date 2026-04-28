/**
 * Email Job Handler Tests
 *
 * Tests for the email-send pg-boss job handler and queue functions.
 */

import { describe, it, expect, vi, beforeEach } from "vitest";
import {
  EMAIL_SEND_JOB,
  RESIDUAL_CALCULATION_JOB,
  type EmailSendJobData,
} from "../jobs/handlers.js";
import {
  enqueueEmail,
  enqueueEmailSimple,
  type EnqueueEmailParams,
} from "../jobs/queue.js";

// ─── Mock Factories ───────────────────────────────────────────────────

function createMockBoss() {
  return {
    work: vi.fn().mockResolvedValue(undefined),
    send: vi.fn().mockResolvedValue("job-id-123"),
    schedule: vi.fn().mockResolvedValue(undefined),
  };
}

function createMockPrisma() {
  return {
    notificationSchedule: {
      create: vi.fn().mockResolvedValue({
        id: "notif-123",
        to: "user@example.com",
        templateId: "merchant-welcome",
        variables: {},
        status: "QUEUED",
        attempts: 0,
        createdAt: new Date(),
        updatedAt: new Date(),
      }),
      update: vi.fn().mockResolvedValue({}),
    },
  };
}

// ─── Tests ────────────────────────────────────────────────────────────

describe("Job Names", () => {
  it("exports EMAIL_SEND_JOB constant", () => {
    expect(EMAIL_SEND_JOB).toBe("email-send");
  });

  it("exports RESIDUAL_CALCULATION_JOB constant", () => {
    expect(RESIDUAL_CALCULATION_JOB).toBe("residual-calculation");
  });
});

describe("enqueueEmail", () => {
  let mockBoss: ReturnType<typeof createMockBoss>;
  let mockPrisma: ReturnType<typeof createMockPrisma>;

  beforeEach(() => {
    mockBoss = createMockBoss();
    mockPrisma = createMockPrisma();
  });

  it("creates NotificationSchedule record and enqueues job", async () => {
    const params: EnqueueEmailParams = {
      to: "user@example.com",
      templateId: "merchant-welcome",
      variables: { merchantName: "Acme Corp" },
      merchantId: "merch-1",
    };

    const notifId = await enqueueEmail(
      mockBoss as any,
      mockPrisma as any,
      params
    );

    expect(notifId).toBe("notif-123");

    // Verify NotificationSchedule created
    expect(mockPrisma.notificationSchedule.create).toHaveBeenCalledOnce();
    const createArgs = mockPrisma.notificationSchedule.create.mock.calls[0][0];
    expect(createArgs.data.to).toBe("user@example.com");
    expect(createArgs.data.templateId).toBe("merchant-welcome");
    expect(createArgs.data.merchantId).toBe("merch-1");
    expect(createArgs.data.status).toBe("QUEUED");

    // Verify pg-boss job enqueued
    expect(mockBoss.send).toHaveBeenCalledOnce();
    const [jobName, jobData, jobOptions] = mockBoss.send.mock.calls[0];
    expect(jobName).toBe("email-send");
    expect(jobData.to).toBe("user@example.com");
    expect(jobData.templateId).toBe("merchant-welcome");
    expect(jobData.notificationId).toBe("notif-123");
    expect(jobOptions.retryLimit).toBe(3);
    expect(jobOptions.retryBackoff).toBe(true);
  });

  it("sets agencyId when provided", async () => {
    const params: EnqueueEmailParams = {
      to: "agency@example.com",
      templateId: "residual-statement",
      variables: { agencyName: "Top Agency" },
      agencyId: "agency-1",
    };

    await enqueueEmail(mockBoss as any, mockPrisma as any, params);

    const createArgs = mockPrisma.notificationSchedule.create.mock.calls[0][0];
    expect(createArgs.data.agencyId).toBe("agency-1");
    expect(createArgs.data.merchantId).toBeNull();
  });

  it("returns null and doesn't throw on failure", async () => {
    mockPrisma.notificationSchedule.create.mockRejectedValueOnce(
      new Error("DB connection error")
    );

    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});

    const result = await enqueueEmail(mockBoss as any, mockPrisma as any, {
      to: "user@example.com",
      templateId: "merchant-welcome",
      variables: {},
    });

    expect(result).toBeNull();
    expect(consoleError).toHaveBeenCalled();

    consoleError.mockRestore();
  });

  it("returns null when boss.send fails", async () => {
    mockBoss.send.mockRejectedValueOnce(new Error("Queue error"));

    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});

    const result = await enqueueEmail(mockBoss as any, mockPrisma as any, {
      to: "user@example.com",
      templateId: "merchant-welcome",
      variables: {},
    });

    expect(result).toBeNull();

    consoleError.mockRestore();
  });
});

describe("enqueueEmailSimple", () => {
  let mockBoss: ReturnType<typeof createMockBoss>;

  beforeEach(() => {
    mockBoss = createMockBoss();
  });

  it("enqueues job without NotificationSchedule", async () => {
    const result = await enqueueEmailSimple(mockBoss as any, {
      to: "user@example.com",
      templateId: "transaction-receipt",
      variables: { amount: "$50.00" },
    });

    expect(result).toBe("job-id-123");
    expect(mockBoss.send).toHaveBeenCalledOnce();

    const [jobName, jobData] = mockBoss.send.mock.calls[0];
    expect(jobName).toBe("email-send");
    expect(jobData.notificationId).toBeUndefined();
  });

  it("returns null on failure without throwing", async () => {
    mockBoss.send.mockRejectedValueOnce(new Error("Queue error"));

    const consoleError = vi
      .spyOn(console, "error")
      .mockImplementation(() => {});

    const result = await enqueueEmailSimple(mockBoss as any, {
      to: "user@example.com",
      templateId: "merchant-welcome",
      variables: {},
    });

    expect(result).toBeNull();

    consoleError.mockRestore();
  });
});

describe("EmailSendJobData type", () => {
  it("accepts valid job data structure", () => {
    const data: EmailSendJobData = {
      to: "user@example.com",
      templateId: "merchant-welcome",
      variables: { merchantName: "Test" },
      merchantId: "merch-1",
      notificationId: "notif-1",
    };

    expect(data.to).toBe("user@example.com");
    expect(data.templateId).toBe("merchant-welcome");
    expect(data.variables.merchantName).toBe("Test");
  });

  it("allows optional fields to be undefined", () => {
    const data: EmailSendJobData = {
      to: "user@example.com",
      templateId: "merchant-welcome",
      variables: {},
    };

    expect(data.merchantId).toBeUndefined();
    expect(data.agencyId).toBeUndefined();
    expect(data.notificationId).toBeUndefined();
  });
});
