import { type QueryClient, useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { $api } from "./client";
import { apiPaths, invalidatePaths, pathKey } from "./paths";

/** The longest username the server takes, in characters. */
export const usernameMaxLength = 64;

/** Who is signed in, with their preferences. */
export const currentUserQueries = {
  current: () => $api.queryOptions("get", "/api/v1/users/me"),
};

/** Whether who is signed in is an admin. */
export function useIsAdmin(): boolean {
  return useSuspenseQuery(currentUserQueries.current()).data.admin;
}

/**
 * Runs once the server has set the session cookie. It drops the previous
 * user's data and fetches the signed-in user afresh.
 */
export function signedIn(queryClient: QueryClient) {
  queryClient.removeQueries();
  return queryClient.fetchQuery({ ...currentUserQueries.current(), staleTime: 0 });
}

/** Change your own preferences. */
export function useUpdateCurrentUser() {
  const queryClient = useQueryClient();
  return $api.useMutation("patch", "/api/v1/users/me", {
    onSuccess: (user) => {
      queryClient.setQueryData(currentUserQueries.current().queryKey, user);
    },
  });
}

/**
 * Change your own password, giving your current one. The list of where
 * you're signed in refetches, because the change may sign out your other
 * sessions.
 */
export function useChangePassword() {
  const queryClient = useQueryClient();
  return $api.useMutation("post", "/api/v1/users/me/change-password", {
    onSuccess: () => queryClient.invalidateQueries({ queryKey: pathKey(apiPaths.sessions) }),
  });
}

// Admin › Users: users and invite links.
export const userQueries = {
  list: () => $api.queryOptions("get", "/api/v1/users"),
  detail: (id: string) => $api.queryOptions("get", "/api/v1/users/{id}", { params: { path: { id } } }),
  invites: () => $api.queryOptions("get", "/api/v1/invites"),
  inviteLink: (token: string) =>
    $api.queryOptions("get", "/api/v1/invite-links/{token}", { params: { path: { token } } }, { retry: false }),
};

/** Refetches the lists and pages of users and invites after any change to them. */
function useInvalidateUsers() {
  const queryClient = useQueryClient();
  return () => invalidatePaths(queryClient, [apiPaths.users, apiPaths.user, apiPaths.invites]);
}

export function useCreateUser() {
  const invalidate = useInvalidateUsers();
  return $api.useMutation("post", "/api/v1/users", { onSuccess: invalidate });
}

/** Change a user. Their page takes the saved user from the answer, and the list refetches. */
export function useUpdateUser() {
  const queryClient = useQueryClient();
  return $api.useMutation("patch", "/api/v1/users/{id}", {
    onSuccess: (user) => {
      queryClient.setQueryData(userQueries.detail(user.id).queryKey, user);
      void invalidatePaths(queryClient, [apiPaths.users]);
    },
  });
}

export function useDeleteUser() {
  const invalidate = useInvalidateUsers();
  return $api.useMutation("delete", "/api/v1/users/{id}", { onSuccess: invalidate });
}

export function useSetUserPassword() {
  return $api.useMutation("post", "/api/v1/users/{id}/set-password");
}

export function useCreateInvite() {
  const invalidate = useInvalidateUsers();
  return $api.useMutation("post", "/api/v1/invites", { onSuccess: invalidate });
}

export function useRevokeInvite() {
  const invalidate = useInvalidateUsers();
  return $api.useMutation("delete", "/api/v1/invites/{id}", { onSuccess: invalidate });
}

/** Accept an invite link. The server creates the user and sets the session cookie. */
export function useAcceptInvite() {
  const queryClient = useQueryClient();
  return $api.useMutation("post", "/api/v1/invite-links/{token}/accept", { onSuccess: () => signedIn(queryClient) });
}
