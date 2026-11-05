import { useQueryClient } from "@tanstack/react-query";
import { $api } from "./client";
import { signedIn } from "./users";

export const setupQueries = {
  state: () => $api.queryOptions("get", "/api/v1/setup"),
};

/** Create the admin user. The server signs this client in. */
export function useCompleteSetup() {
  const queryClient = useQueryClient();
  return $api.useMutation("post", "/api/v1/setup", {
    onSuccess: () => {
      queryClient.setQueryData(setupQueries.state().queryKey, (s) => (s ? { ...s, required: false } : s));
      return signedIn(queryClient);
    },
  });
}
