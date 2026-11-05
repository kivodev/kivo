import { expect, it } from "vitest";
import { http, sent, serveMockApi, server } from "@/test/server";
import { client } from "./client";
import { reportClientError, startClientErrorReports } from "./clientErrors";

it("sends the TV build's errors to the server once signed in", async () => {
  serveMockApi();
  server.use(http.post("/api/v1/client-errors", ({ response }) => response(204).empty()));
  startClientErrorReports();
  reportClientError("error", new Error("before sign-in"));
  await client.GET("/api/v1/users/me");
  reportClientError("error", new Error("boom"));
  await expect.poll(() => sent("POST", "/client-errors")).toHaveLength(1);
  expect(sent("POST", "/client-errors")[0]?.body).toMatchObject({ kind: "error", message: "Error: boom" });
});
