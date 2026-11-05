import { useQueryClient } from "@tanstack/react-query";
import { $api } from "./client";
import { apiPaths, invalidatePaths } from "./paths";

export const settingsQueries = {
  current: () => $api.queryOptions("get", "/api/v1/settings"),
};

export function useUpdateSettings() {
  const queryClient = useQueryClient();
  return $api.useMutation("patch", "/api/v1/settings", {
    onSuccess: (settings) => {
      queryClient.setQueryData(settingsQueries.current().queryKey, settings);
      // The night tasks' next runs follow the maintenance window.
      void invalidatePaths(queryClient, [apiPaths.tasks]);
    },
  });
}
