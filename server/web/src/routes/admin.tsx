import { createRoute, lazyRouteComponent, notFound } from "@tanstack/react-router";
import { z } from "zod";
import { activityQueries } from "@/api/activity";
import { apiKeyQueries } from "@/api/apiKeys";
import { fileQueries } from "@/api/files";
import { historyQueries } from "@/api/history";
import { libraryQueries } from "@/api/libraries";
import { logQueries } from "@/api/logs";
import { acceleratorQueries } from "@/api/playback";
import { problemQueries } from "@/api/problems";
import { sessionQueries } from "@/api/session";
import { settingsQueries } from "@/api/settings";
import { taskQueries } from "@/api/tasks";
import { userQueries } from "@/api/users";
import { workQueries } from "@/api/works";
import { authedRoute } from "./authed";

/** The web-only admin tree is made only when the router includes it. */
export function createAdminRouteTree() {
  // Read directly rather than isTv, so the TV build drops the screens below.
  if (import.meta.env.VITE_KIVO_TV === "1") throw new Error("Admin routes are web-only");
  const AdminLayout = lazyRouteComponent(() => import("@/features/admin/AdminLayout"), "AdminLayout");
  const OverviewScreen = lazyRouteComponent(() => import("@/features/admin/OverviewScreen"), "OverviewScreen");
  const ManageLibrariesScreen = lazyRouteComponent(
    () => import("@/features/admin/ManageLibrariesScreen"),
    "ManageLibrariesScreen",
  );
  const MissingFilesScreen = lazyRouteComponent(
    () => import("@/features/admin/MissingFilesScreen"),
    "MissingFilesScreen",
  );
  const UnmatchedScreen = lazyRouteComponent(() => import("@/features/admin/UnmatchedScreen"), "UnmatchedScreen");
  const TasksScreen = lazyRouteComponent(() => import("@/features/admin/TasksScreen"), "TasksScreen");
  const TaskScreen = lazyRouteComponent(() => import("@/features/admin/TaskScreen"), "TaskScreen");
  const ProblemsScreen = lazyRouteComponent(() => import("@/features/admin/ProblemsScreen"), "ProblemsScreen");
  const LogsScreen = lazyRouteComponent(() => import("@/features/admin/LogsScreen"), "LogsScreen");
  const ActivityScreen = lazyRouteComponent(() => import("@/features/admin/ActivityScreen"), "ActivityScreen");
  const ServerHistoryScreen = lazyRouteComponent(
    () => import("@/features/history/HistoryScreen"),
    "ServerHistoryScreen",
  );
  const UsersScreen = lazyRouteComponent(() => import("@/features/admin/UsersScreen"), "UsersScreen");
  const UserScreen = lazyRouteComponent(() => import("@/features/admin/UserScreen"), "UserScreen");
  const GeneralSettingsScreen = lazyRouteComponent(
    () => import("@/features/admin/GeneralSettingsScreen"),
    "GeneralSettingsScreen",
  );
  const LibrarySettingsScreen = lazyRouteComponent(
    () => import("@/features/admin/LibrarySettingsScreen"),
    "LibrarySettingsScreen",
  );
  const TranscodingScreen = lazyRouteComponent(() => import("@/features/admin/TranscodingScreen"), "TranscodingScreen");
  const NetworkSettingsScreen = lazyRouteComponent(
    () => import("@/features/admin/NetworkSettingsScreen"),
    "NetworkSettingsScreen",
  );
  const ApiKeysScreen = lazyRouteComponent(() => import("@/features/admin/ApiKeysScreen"), "ApiKeysScreen");

  // The user and the list of libraries belong to the signed-in routes and
  // are already loaded.

  /** The admin area, for admins only. To anyone else it doesn't exist. */
  const adminRoute = createRoute({
    getParentRoute: () => authedRoute,
    path: "/admin",
    beforeLoad: ({ context }) => {
      if (!context.user.admin) throw notFound();
    },
    component: AdminLayout,
  });

  const overviewRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/",
    loader: ({ context: { queryClient } }) => {
      void queryClient.prefetchQuery(problemQueries.list());
      void queryClient.prefetchQuery(activityQueries.latest());
    },
    component: OverviewScreen,
  });

  const librariesRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/libraries",
    loader: ({ context }) => void context.queryClient.prefetchQuery(libraryQueries.admin()),
    component: ManageLibrariesScreen,
  });

  const missingFilesRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/missing-files",
    loader: ({ context }) => void context.queryClient.prefetchInfiniteQuery(fileQueries.missing()),
    component: MissingFilesScreen,
  });

  const unmatchedRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/unmatched",
    loader: ({ context }) => void context.queryClient.prefetchInfiniteQuery(workQueries.unmatched()),
    component: UnmatchedScreen,
  });

  const tasksRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/tasks",
    loader: ({ context: { queryClient } }) => {
      void queryClient.prefetchQuery(taskQueries.list());
      void queryClient.prefetchQuery(settingsQueries.current());
    },
    component: TasksScreen,
  });

  /** The page of a task that keeps failures. Other tasks have no page. */
  const taskRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/tasks/$task",
    loader: async ({ context: { queryClient }, params }) => {
      // This waits, because whether the task keeps failures decides whether
      // its page exists.
      const tasks = await queryClient.ensureQueryData(taskQueries.list());
      const task = tasks.items.find((t) => t.id === params.task && t.failedCount !== undefined);
      if (!task) throw notFound();
      void queryClient.prefetchQuery(taskQueries.failures(task.id));
    },
    component: TaskScreen,
  });

  const problemsRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/problems",
    loader: ({ context }) => void context.queryClient.prefetchQuery(problemQueries.list()),
    component: ProblemsScreen,
  });

  const logsRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/logs",
    loader: ({ context }) => void context.queryClient.prefetchQuery(logQueries.list("info", "")),
    component: LogsScreen,
  });

  /** The Activity page's URL state: the Show filter. */
  const activitySearchSchema = z.object({
    show: z.enum(["plays", "added", "signIns", "server"]).optional().catch(undefined),
  });

  const activityRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/activity",
    validateSearch: activitySearchSchema,
    loaderDeps: ({ search }) => ({ show: search.show }),
    loader: ({ context, deps }) => void context.queryClient.prefetchInfiniteQuery(activityQueries.list(deps.show)),
    component: ActivityScreen,
  });

  /** The server-wide watch history's URL state, which is the user it's filtered to. */
  const serverHistorySearchSchema = z.object({ userId: z.string().min(1).max(100).optional().catch(undefined) });

  const serverHistoryRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/history",
    validateSearch: serverHistorySearchSchema,
    loaderDeps: ({ search }) => ({ userId: search.userId }),
    loader: ({ context: { queryClient }, deps }) => {
      void queryClient.prefetchInfiniteQuery(historyQueries.everyone(deps.userId));
      void queryClient.prefetchQuery(userQueries.list());
    },
    component: ServerHistoryScreen,
  });

  const usersRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/users",
    loader: ({ context: { queryClient } }) => {
      void queryClient.prefetchQuery(userQueries.list());
      void queryClient.prefetchQuery(userQueries.invites());
    },
    component: UsersScreen,
  });

  const userRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/users/$userId",
    loader: ({ context: { queryClient }, params }) => {
      void queryClient.prefetchQuery(userQueries.detail(params.userId));
      void queryClient.prefetchQuery(sessionQueries.forUser(params.userId));
      void queryClient.prefetchQuery(workQueries.contentRatings());
    },
    component: UserScreen,
  });

  // Admin › Settings, in the sidebar's order.
  const generalSettingsRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/settings/general",
    loader: ({ context }) => void context.queryClient.prefetchQuery(settingsQueries.current()),
    component: GeneralSettingsScreen,
  });

  const librarySettingsRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/settings/library",
    loader: ({ context }) => void context.queryClient.prefetchQuery(settingsQueries.current()),
    component: LibrarySettingsScreen,
  });

  const transcodingSettingsRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/settings/transcoding",
    loader: ({ context: { queryClient } }) => {
      void queryClient.prefetchQuery(settingsQueries.current());
      void queryClient.prefetchQuery(acceleratorQueries.list());
    },
    component: TranscodingScreen,
  });

  const networkSettingsRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/settings/network",
    loader: ({ context }) => void context.queryClient.prefetchQuery(settingsQueries.current()),
    component: NetworkSettingsScreen,
  });

  const apiKeysRoute = createRoute({
    getParentRoute: () => adminRoute,
    path: "/settings/api-keys",
    loader: ({ context }) => void context.queryClient.prefetchQuery(apiKeyQueries.list()),
    component: ApiKeysScreen,
  });

  /** The admin routes, for the router's tree. */
  return adminRoute.addChildren([
    overviewRoute,
    librariesRoute,
    missingFilesRoute,
    unmatchedRoute,
    tasksRoute,
    taskRoute,
    problemsRoute,
    logsRoute,
    activityRoute,
    serverHistoryRoute,
    usersRoute,
    userRoute,
    generalSettingsRoute,
    librarySettingsRoute,
    transcodingSettingsRoute,
    networkSettingsRoute,
    apiKeysRoute,
  ]);
}
