import { useQueryClient } from "@tanstack/react-query";
import { $api } from "./client";

// Admin › API keys: keys for scripts and other tools.
export const apiKeyQueries = {
  list: () => $api.queryOptions("get", "/api/v1/api-keys"),
};

export function useCreateApiKey() {
  const queryClient = useQueryClient();
  return $api.useMutation("post", "/api/v1/api-keys", {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: apiKeyQueries.list().queryKey }),
  });
}

export function useRevokeApiKey() {
  const queryClient = useQueryClient();
  return $api.useMutation("delete", "/api/v1/api-keys/{id}", {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: apiKeyQueries.list().queryKey }),
  });
}
