import { waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderApp, screen } from "@/test/render";
import { sent, serveMockApi } from "@/test/server";

describe("Settings › Network", () => {
  it("sets the remote streaming limit for users without one of their own", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/settings/network");
    await user.click(await screen.findByRole("button", { name: /Remote streaming limit/ }));
    await user.click(await screen.findByRole("option", { name: /8 Mb\/s/ }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(sent("PATCH", "/settings")[0]?.body).toMatchObject({ remoteQualityCapBps: 8_000_000 }));
  });
});
