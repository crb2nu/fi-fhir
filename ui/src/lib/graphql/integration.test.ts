import { afterEach, beforeEach, describe, it, expect } from "vitest";
import {
  EventsDocument,
  HealthDocument,
  PreviewIntegrationMessageDocument,
} from "$lib/gen/graphql";
import { graphqlFetch } from "./client";
import { setGraphQLCredentialProvider } from "./credentials";

describe("Live Backend GraphQL Integration", () => {
  const backendUrl =
    (typeof process !== "undefined" && process.env.VITE_GRAPHQL_API_URL) ||
    "http://localhost:8080/graphql";

  // These tests require a running fi-fhir backend.
  const isCi =
    typeof process !== "undefined" &&
    process.env.CI_INTEGRATION_TEST === "true";

  const bearerToken =
    (typeof process !== "undefined" &&
      process.env.FI_FHIR_GRAPHQL_BEARER_TOKEN) ||
    "";
  const previewIntegrationId =
    (typeof process !== "undefined" &&
      process.env.FI_FHIR_PREVIEW_INTEGRATION_ID) ||
    "adt-east";

  (isCi ? describe : describe.skip)("Query execution", () => {
    beforeEach(() => {
      if (!bearerToken) {
        throw new Error("FI_FHIR_GRAPHQL_BEARER_TOKEN is required for live UI tests");
      }
      setGraphQLCredentialProvider(() => bearerToken);
    });

    afterEach(() => {
      setGraphQLCredentialProvider(null);
    });

    it("should reject non-preview operations for the preview-only bearer", async () => {
      const originalFetch = globalThis.fetch;
      const liveFetch = (
        _url: RequestInfo | URL,
        init?: RequestInit,
      ): Promise<Response> => {
        return originalFetch(backendUrl, init);
      };

      globalThis.fetch = liveFetch as unknown as typeof fetch;

      try {
        await expect(
          graphqlFetch(
            EventsDocument,
            {
              first: 1,
              filter: null,
              after: null,
              orderBy: null,
            },
            { showErrorToast: false },
          ),
        ).rejects.toThrow("GraphQL operation forbidden");
      } finally {
        globalThis.fetch = originalFetch;
      }
    });

    it("should successfully execute HealthDocument against live backend", async () => {
      const originalFetch = globalThis.fetch;
      const liveFetch = (
        _url: RequestInfo | URL,
        init?: RequestInit,
      ): Promise<Response> => {
        return originalFetch(backendUrl, init);
      };

      globalThis.fetch = liveFetch as unknown as typeof fetch;

      try {
        const result = await graphqlFetch(HealthDocument, {}, {
          showErrorToast: false,
        });

        expect(result).toBeDefined();
        expect(result?.health.status).toBe("healthy");
      } finally {
        globalThis.fetch = originalFetch;
      }
    });

    it("should execute the authenticated stateless preview mutation", async () => {
      const originalFetch = globalThis.fetch;
      const liveFetch = (
        _url: RequestInfo | URL,
        init?: RequestInit,
      ): Promise<Response> => originalFetch(backendUrl, init);

      globalThis.fetch = liveFetch as unknown as typeof fetch;

      try {
        const result = await graphqlFetch(
          PreviewIntegrationMessageDocument,
          {
            input: {
              integrationId: previewIntegrationId,
              data:
                "MSH|^~\\&|UI-LIVE|FAC|FI-FHIR|FAC|20260713120000||ADT^A01|ui-live-1|P|2.5.1\r" +
                "EVN|A01|20260713120000||||20260713115900-0400\r" +
                "PID|1||MRN-UI-LIVE^^^HOSP^MR||Patient^Preview||19800101|F\r" +
                "PV1|1|I|UNIT^101^A^FAC||||||||||||||||visit-ui-live|||||||||||||||||||||||||20260713120000",
              correlationId: "ui-live-correlation-1",
              reason: "live UI transport verification",
            },
          },
          { showErrorToast: false },
        );

        expect(result.previewIntegrationMessage.mode).toBe("preview");
        expect(result.previewIntegrationMessage.events).toHaveLength(1);
        expect(JSON.stringify(result.previewIntegrationMessage)).not.toContain("MSH|");
      } finally {
        globalThis.fetch = originalFetch;
      }
    });
  });
});
