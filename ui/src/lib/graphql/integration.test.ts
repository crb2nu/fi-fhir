import { describe, it, expect } from "vitest";
import { EventsDocument, LookupMappingDocument } from "$lib/gen/graphql";
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
      // Create a specific fetch implementation to hit the live backend url
      const liveFetch = (
        url: RequestInfo | URL,
        init?: RequestInit,
      ): Promise<Response> => {
        return fetch(backendUrl, init);
      };

      const originalFetch = globalThis.fetch;
      globalThis.fetch = liveFetch as any;

      try {
        const result = await graphqlFetch(
          EventsDocument,
          {
            first: 1,
            filter: {} as any,
            after: null as any,
            orderBy: null as any,
          },
          { showErrorToast: false },
        );

        expect(result).toBeDefined();
        expect((result as any).events).toBeDefined();
      } finally {
        globalThis.fetch = originalFetch;
      }
    });

    it("should successfully execute LookupMappingDocument against live backend", async () => {
      const liveFetch = (
        url: RequestInfo | URL,
        init?: RequestInit,
      ): Promise<Response> => {
        return fetch(backendUrl, init);
      };

      const originalFetch = globalThis.fetch;
      globalThis.fetch = liveFetch as any;

      try {
        const result = await graphqlFetch(
          LookupMappingDocument,
          {
            // providing variables that terminology query needs
            sourceSystem: "http://loinc.org",
            sourceCode: "1234-5",
            targetSystem: "http://snomed.info/sct",
            profileId: null,
          },
          { showErrorToast: false },
        );

        expect(result).toBeDefined();
        // Just verify it parses cleanly from the live schema
      } finally {
        globalThis.fetch = originalFetch;
      }
    });
  });
});
