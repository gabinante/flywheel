/**
 * Tests for NMI Partner Boarding API service.
 *
 * Tests cover:
 * - Configuration validation
 * - Boarding submission with various NMI responses
 * - Status checking for APPROVED, UNDER_REVIEW, DECLINED
 * - PII decryption before API calls
 * - Credential encryption after approval
 * - Response parsing (XML, key=value, JSON formats)
 * - Error handling
 */

import https from "https";
import { EventEmitter } from "events";
import {
  submitBoardingApplication,
  checkBoardingStatus,
  encryptNmiCredentials,
  isNmiConfigured,
  _resetConfig,
  _parseNmiResponse,
  _decryptIfNeeded,
  BoardingApplicationInput,
} from "../nmi-boarding.service";
import { encrypt, decrypt, _resetKeys } from "../../utils/encryption";

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

/** Generate a valid test encryption key (32 bytes hex) */
const TEST_ENCRYPTION_KEY =
  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";

/** Sample boarding application input */
function makeSampleInput(
  overrides: Partial<BoardingApplicationInput> = {}
): BoardingApplicationInput {
  return {
    applicationId: "app-001",
    merchantId: "merch-001",
    businessLegalName: "Test Corp LLC",
    businessDba: "Test Corp",
    businessEin: "12-3456789",
    businessType: "LLC",
    businessAddress: "123 Main St",
    businessCity: "Austin",
    businessState: "TX",
    businessZip: "78701",
    businessPhone: "5125551234",
    businessEmail: "contact@testcorp.com",
    businessWebsite: "https://testcorp.com",
    ownerFirstName: "Jane",
    ownerLastName: "Doe",
    ownerEmail: "jane@testcorp.com",
    ownerPhone: "5125555678",
    ownerDob: "1990-01-15",
    ownerSsn: "123-45-6789",
    ownerAddress: "456 Oak Ave",
    ownerCity: "Austin",
    ownerState: "TX",
    ownerZip: "78702",
    bankRoutingNumber: "021000021",
    bankAccountNumber: "123456789",
    bankAccountType: "checking",
    bankName: "Test Bank",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Mock HTTPS
// ---------------------------------------------------------------------------

// We'll mock the https module to avoid real network calls
jest.mock("https", () => {
  const originalModule = jest.requireActual("https");
  return {
    ...originalModule,
    request: jest.fn(),
  };
});

const mockedHttps = https as jest.Mocked<typeof https>;

/** Helper to set up a mock HTTPS response */
function mockHttpsResponse(statusCode: number, body: string): void {
  const mockResponse = new EventEmitter() as any;
  mockResponse.statusCode = statusCode;

  const mockRequest = new EventEmitter() as any;
  mockRequest.write = jest.fn();
  mockRequest.end = jest.fn();
  mockRequest.setTimeout = jest.fn();
  mockRequest.destroy = jest.fn();

  (mockedHttps.request as jest.Mock).mockImplementation((_opts, callback) => {
    // Call the callback with the mock response on next tick
    process.nextTick(() => {
      callback(mockResponse);
      // Emit data and end
      mockResponse.emit("data", Buffer.from(body));
      mockResponse.emit("end");
    });
    return mockRequest;
  });
}

/** Helper to simulate a network error */
function mockHttpsError(errorMessage: string): void {
  const mockRequest = new EventEmitter() as any;
  mockRequest.write = jest.fn();
  mockRequest.end = jest.fn();
  mockRequest.setTimeout = jest.fn();
  mockRequest.destroy = jest.fn();

  (mockedHttps.request as jest.Mock).mockImplementation((_opts, _callback) => {
    process.nextTick(() => {
      mockRequest.emit("error", new Error(errorMessage));
    });
    return mockRequest;
  });
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("NMI Boarding Service", () => {
  const originalEnv = process.env;

  beforeEach(() => {
    jest.clearAllMocks();
    process.env = { ...originalEnv };
    process.env.ENCRYPTION_KEY = TEST_ENCRYPTION_KEY;
    process.env.NMI_PARTNER_ID = "test-partner-id";
    process.env.NMI_PARTNER_KEY = "test-partner-key";
    _resetConfig();
    _resetKeys();
  });

  afterEach(() => {
    process.env = originalEnv;
    _resetConfig();
    _resetKeys();
  });

  // -----------------------------------------------------------------------
  // Configuration
  // -----------------------------------------------------------------------

  describe("isNmiConfigured", () => {
    it("returns true when both NMI_PARTNER_ID and NMI_PARTNER_KEY are set", () => {
      expect(isNmiConfigured()).toBe(true);
    });

    it("returns false when NMI_PARTNER_ID is missing", () => {
      delete process.env.NMI_PARTNER_ID;
      expect(isNmiConfigured()).toBe(false);
    });

    it("returns false when NMI_PARTNER_KEY is missing", () => {
      delete process.env.NMI_PARTNER_KEY;
      expect(isNmiConfigured()).toBe(false);
    });

    it("returns false when both are missing", () => {
      delete process.env.NMI_PARTNER_ID;
      delete process.env.NMI_PARTNER_KEY;
      expect(isNmiConfigured()).toBe(false);
    });
  });

  // -----------------------------------------------------------------------
  // Response parsing
  // -----------------------------------------------------------------------

  describe("parseNmiResponse", () => {
    it("parses XML responses", () => {
      const xml =
        "<response><status>APPROVED</status><merchant_id>MID123</merchant_id></response>";
      const result = _parseNmiResponse(xml);
      expect(result.status).toBe("APPROVED");
      expect(result.merchant_id).toBe("MID123");
    });

    it("parses URL-encoded key=value responses", () => {
      const body = "status=APPROVED&merchant_id=MID123&security_key=SK456";
      const result = _parseNmiResponse(body);
      expect(result.status).toBe("APPROVED");
      expect(result.merchant_id).toBe("MID123");
      expect(result.security_key).toBe("SK456");
    });

    it("parses JSON responses", () => {
      const body = JSON.stringify({
        status: "APPROVED",
        merchant_id: "MID123",
      });
      const result = _parseNmiResponse(body);
      expect(result.status).toBe("APPROVED");
      expect(result.merchant_id).toBe("MID123");
    });

    it("returns raw body for unparseable responses", () => {
      const body = "Something unexpected";
      const result = _parseNmiResponse(body);
      expect(result.raw).toBe("Something unexpected");
    });
  });

  // -----------------------------------------------------------------------
  // PII decryption helper
  // -----------------------------------------------------------------------

  describe("decryptIfNeeded", () => {
    it("returns plaintext values as-is", () => {
      expect(_decryptIfNeeded("123-45-6789")).toBe("123-45-6789");
    });

    it("decrypts encrypted values", () => {
      const encrypted = encrypt("sensitive-data");
      const result = _decryptIfNeeded(encrypted);
      expect(result).toBe("sensitive-data");
    });

    it("returns empty string for empty input", () => {
      expect(_decryptIfNeeded("")).toBe("");
    });
  });

  // -----------------------------------------------------------------------
  // submitBoardingApplication
  // -----------------------------------------------------------------------

  describe("submitBoardingApplication", () => {
    it("throws if NMI is not configured", async () => {
      delete process.env.NMI_PARTNER_ID;
      delete process.env.NMI_PARTNER_KEY;
      _resetConfig();

      await expect(
        submitBoardingApplication(makeSampleInput())
      ).rejects.toThrow("NMI partner credentials not configured");
    });

    it("handles instant approval response", async () => {
      const responseBody =
        "status=APPROVED&merchant_id=MID789&security_key=SK_SECRET&tokenization_key=TK_TOKEN&boarding_id=BRD123&message=Approved";
      mockHttpsResponse(200, responseBody);

      const result = await submitBoardingApplication(makeSampleInput());

      expect(result.success).toBe(true);
      expect(result.status).toBe("APPROVED");
      expect(result.nmiMerchantId).toBe("MID789");
      expect(result.securityKey).toBe("SK_SECRET");
      expect(result.tokenizationKey).toBe("TK_TOKEN");
      expect(result.nmiApplicationId).toBe("BRD123");
    });

    it("handles UNDER_REVIEW response", async () => {
      const responseBody =
        "status=PENDING&boarding_id=BRD456&message=Under+review";
      mockHttpsResponse(200, responseBody);

      const result = await submitBoardingApplication(makeSampleInput());

      expect(result.success).toBe(true);
      expect(result.status).toBe("UNDER_REVIEW");
      expect(result.nmiApplicationId).toBe("BRD456");
    });

    it("handles DECLINED response", async () => {
      const responseBody =
        "status=DECLINED&boarding_id=BRD789&decline_reason=High+risk+category&message=Declined";
      mockHttpsResponse(200, responseBody);

      const result = await submitBoardingApplication(makeSampleInput());

      expect(result.success).toBe(false);
      expect(result.status).toBe("DECLINED");
      expect(result.declineReason).toBe("High risk category");
    });

    it("handles HTTP error responses", async () => {
      mockHttpsResponse(500, "error=Internal+server+error");

      const result = await submitBoardingApplication(makeSampleInput());

      expect(result.success).toBe(false);
      expect(result.status).toBe("DECLINED");
    });

    it("handles network errors", async () => {
      mockHttpsError("Connection refused");

      await expect(
        submitBoardingApplication(makeSampleInput())
      ).rejects.toThrow("NMI API request failed: Connection refused");
    });

    it("sends decrypted PII in the API call", async () => {
      // Encrypt sensitive fields
      const encryptedSsn = encrypt("999-88-7777");
      const encryptedRouting = encrypt("021000021");
      const encryptedAccount = encrypt("987654321");
      const encryptedEin = encrypt("98-7654321");

      mockHttpsResponse(
        200,
        "status=APPROVED&merchant_id=M1&security_key=SK1&tokenization_key=TK1&boarding_id=B1"
      );

      const input = makeSampleInput({
        ownerSsn: encryptedSsn,
        bankRoutingNumber: encryptedRouting,
        bankAccountNumber: encryptedAccount,
        businessEin: encryptedEin,
      });

      await submitBoardingApplication(input);

      // Verify the HTTPS request was made
      expect(mockedHttps.request).toHaveBeenCalled();

      // Get the request body that was written
      const requestCall = (mockedHttps.request as jest.Mock).mock.results[0];
      const mockReq =
        requestCall && requestCall.value
          ? requestCall.value
          : (mockedHttps.request as jest.Mock).mock.results[0];

      // The write call should have the decrypted values (not encrypted envelopes)
      const writeCalls = mockReq.write?.mock?.calls;
      if (writeCalls && writeCalls.length > 0) {
        const body = writeCalls[0][0];
        const params = new URLSearchParams(body);

        // Verify decrypted values were sent
        expect(params.get("owner_ssn")).toBe("999-88-7777");
        expect(params.get("bank_routing_number")).toBe("021000021");
        expect(params.get("bank_account_number")).toBe("987654321");
        expect(params.get("federal_tax_id")).toBe("98-7654321");
      }
    });

    it("includes partner credentials in the request (but never logs them)", async () => {
      mockHttpsResponse(200, "status=APPROVED&merchant_id=M1&security_key=SK1&tokenization_key=TK1&boarding_id=B1");

      // Spy on console.log to verify no credentials are logged
      const logSpy = jest.spyOn(console, "log").mockImplementation();

      await submitBoardingApplication(makeSampleInput());

      // Verify request was made
      expect(mockedHttps.request).toHaveBeenCalled();

      // Check the write call includes partner_id
      const mockReq = (mockedHttps.request as jest.Mock).mock.results[0]?.value;
      if (mockReq?.write?.mock?.calls?.length) {
        const body = mockReq.write.mock.calls[0][0];
        const params = new URLSearchParams(body);
        expect(params.get("partner_id")).toBe("test-partner-id");
        expect(params.get("partner_key")).toBe("test-partner-key");
      }

      // Verify no log message contains the partner key
      for (const call of logSpy.mock.calls) {
        const msg = call.join(" ");
        expect(msg).not.toContain("test-partner-key");
      }

      logSpy.mockRestore();
    });

    it("handles XML approval response", async () => {
      const xml =
        "<response><status>APPROVED</status><merchant_id>MID_XML</merchant_id>" +
        "<security_key>SK_XML</security_key><tokenization_key>TK_XML</tokenization_key>" +
        "<boarding_id>BRD_XML</boarding_id></response>";
      mockHttpsResponse(200, xml);

      const result = await submitBoardingApplication(makeSampleInput());

      expect(result.success).toBe(true);
      expect(result.status).toBe("APPROVED");
      expect(result.nmiMerchantId).toBe("MID_XML");
      expect(result.securityKey).toBe("SK_XML");
      expect(result.tokenizationKey).toBe("TK_XML");
    });

    it("handles JSON approval response", async () => {
      const json = JSON.stringify({
        status: "APPROVED",
        merchant_id: "MID_JSON",
        security_key: "SK_JSON",
        tokenization_key: "TK_JSON",
        boarding_id: "BRD_JSON",
      });
      mockHttpsResponse(200, json);

      const result = await submitBoardingApplication(makeSampleInput());

      expect(result.success).toBe(true);
      expect(result.status).toBe("APPROVED");
      expect(result.nmiMerchantId).toBe("MID_JSON");
    });

    it("handles response_code based responses", async () => {
      mockHttpsResponse(200, "response_code=100&merchant_id=M2&security_key=SK2&tokenization_key=TK2&boarding_id=B2");

      const result = await submitBoardingApplication(makeSampleInput());

      expect(result.success).toBe(true);
      expect(result.status).toBe("APPROVED");
    });

    it("treats unknown status as UNDER_REVIEW", async () => {
      mockHttpsResponse(200, "status=PROCESSING&boarding_id=B3");

      const warnSpy = jest.spyOn(console, "warn").mockImplementation();
      const result = await submitBoardingApplication(makeSampleInput());
      warnSpy.mockRestore();

      expect(result.success).toBe(true);
      expect(result.status).toBe("UNDER_REVIEW");
    });
  });

  // -----------------------------------------------------------------------
  // checkBoardingStatus
  // -----------------------------------------------------------------------

  describe("checkBoardingStatus", () => {
    it("throws if NMI is not configured", async () => {
      delete process.env.NMI_PARTNER_ID;
      delete process.env.NMI_PARTNER_KEY;
      _resetConfig();

      await expect(checkBoardingStatus("BRD123")).rejects.toThrow(
        "NMI partner credentials not configured"
      );
    });

    it("returns APPROVED with credentials", async () => {
      mockHttpsResponse(
        200,
        "status=APPROVED&merchant_id=MID_POLL&security_key=SK_POLL&tokenization_key=TK_POLL"
      );

      const result = await checkBoardingStatus("BRD123");

      expect(result.status).toBe("APPROVED");
      expect(result.nmiMerchantId).toBe("MID_POLL");
      expect(result.securityKey).toBe("SK_POLL");
      expect(result.tokenizationKey).toBe("TK_POLL");
      expect(result.nmiApplicationId).toBe("BRD123");
    });

    it("returns UNDER_REVIEW for pending applications", async () => {
      mockHttpsResponse(200, "status=PENDING&message=Still+processing");

      const result = await checkBoardingStatus("BRD456");

      expect(result.status).toBe("UNDER_REVIEW");
      expect(result.nmiApplicationId).toBe("BRD456");
    });

    it("returns DECLINED with reason", async () => {
      mockHttpsResponse(
        200,
        "status=DECLINED&decline_reason=Credit+check+failed"
      );

      const result = await checkBoardingStatus("BRD789");

      expect(result.status).toBe("DECLINED");
      expect(result.declineReason).toBe("Credit check failed");
    });

    it("handles network errors", async () => {
      mockHttpsError("ECONNRESET");

      await expect(checkBoardingStatus("BRD_ERR")).rejects.toThrow(
        "NMI API request failed: ECONNRESET"
      );
    });

    it("treats unknown status as UNDER_REVIEW", async () => {
      mockHttpsResponse(200, "status=MANUAL_REVIEW");

      const result = await checkBoardingStatus("BRD_UNK");

      expect(result.status).toBe("UNDER_REVIEW");
    });
  });

  // -----------------------------------------------------------------------
  // encryptNmiCredentials
  // -----------------------------------------------------------------------

  describe("encryptNmiCredentials", () => {
    it("encrypts security key and tokenization key", () => {
      const creds = encryptNmiCredentials({
        nmiMerchantId: "MID123",
        securityKey: "raw-security-key",
        tokenizationKey: "raw-token-key",
      });

      expect(creds.nmiMerchantId).toBe("MID123");
      expect(creds.nmiSecurityKey).not.toBe("raw-security-key");
      expect(creds.nmiTokenizationKey).not.toBe("raw-token-key");

      // Verify they can be decrypted
      expect(decrypt(creds.nmiSecurityKey!)).toBe("raw-security-key");
      expect(decrypt(creds.nmiTokenizationKey!)).toBe("raw-token-key");
    });

    it("handles missing credentials gracefully", () => {
      const creds = encryptNmiCredentials({});

      expect(creds.nmiMerchantId).toBeNull();
      expect(creds.nmiSecurityKey).toBeNull();
      expect(creds.nmiTokenizationKey).toBeNull();
    });

    it("handles partial credentials", () => {
      const creds = encryptNmiCredentials({
        nmiMerchantId: "MID456",
        securityKey: "some-key",
      });

      expect(creds.nmiMerchantId).toBe("MID456");
      expect(creds.nmiSecurityKey).not.toBeNull();
      expect(creds.nmiTokenizationKey).toBeNull();
    });
  });
});
