import { describe, it, expect } from "vitest";
import { EventsDocument, HealthDocument } from "$lib/gen/graphql";
import { graphqlFetch } from "./client";

describe("Live Backend GraphQL Integration", () => {
  const backendUrl =
    (typeof process !== "undefined" && process.env.VITE_GRAPHQL_API_URL) ||
    "http://localhost:8080/graphql";

  // These tests require a running fi-fhir backend.
  const isCi =
    typeof process !== "undefined" &&
    process.env.CI_INTEGRATION_TEST === "true";

  (isCi ? describe : describe.skip)("Query execution", () => {
    it("should successfully execute EventsDocument against live backend", async () => {
      const originalFetch = globalThis.fetch;
      const liveFetch = (
        _url: RequestInfo | URL,
        init?: RequestInit,
      ): Promise<Response> => {
        return originalFetch(backendUrl, init);
      };

      globalThis.fetch = liveFetch as unknown as typeof fetch;

      try {
        const result = await graphqlFetch(
          EventsDocument,
          {
            first: 1,
            filter: null,
            after: null,
            orderBy: null,
          },
          { showErrorToast: false },
        );

        expect(result).toBeDefined();
        expect(result?.events).toBeDefined();
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
  });
});
