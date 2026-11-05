import { QueryClient } from "@tanstack/react-query";
import { shouldRetry } from "@/api/problem";

export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: shouldRetry,
        // Live events invalidate what changed, so data stays fresh without polling.
        staleTime: 30_000,
      },
      mutations: { retry: false },
    },
  });
}
