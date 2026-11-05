import { useQueryClient } from "@tanstack/react-query";
import { $api } from "./client";
import { apiPaths, pathKey } from "./paths";

export const problemQueries = {
  /** Open problems, newest first. */
  list: () => $api.queryOptions("get", "/api/v1/problems"),
};

/** Hide a problem until it resolves and comes back. */
export function useDismissProblem() {
  const queryClient = useQueryClient();
  return $api.useMutation("delete", "/api/v1/problems/{id}", {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: pathKey(apiPaths.problems) }),
  });
}
