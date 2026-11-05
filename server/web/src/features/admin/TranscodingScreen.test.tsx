import { waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderApp, screen } from "@/test/render";
import { onRequest, sent, serveMockApi } from "@/test/server";

describe("Settings › Transcoding", () => {
  it("offers the accelerators the test passed, and gives the fix for one it couldn't use", async () => {
    serveMockApi();
    const { user } = renderApp("/admin/settings/transcoding");
    expect(await screen.findByRole("heading", { name: "Hardware acceleration" })).toBeInTheDocument();
    const problem = await screen.findByText("NVIDIA GeForce GTX 1060");
    expect(problem.closest('[data-part="accelerator-problem"]')).toBeInTheDocument();
    expect(document.querySelectorAll('[data-part="accelerator-problem"]')).toHaveLength(1);
    expect(screen.getByText("Driver not found.")).toBeInTheDocument();
    expect(
      screen.getByText("Install the driver on the host. For NVIDIA, use the NVIDIA container runtime."),
    ).toBeInTheDocument();
    expect(screen.getByText("libcuda.so.1: cannot open shared object file")).toBeInTheDocument();
    // Each usable accelerator says what it does, naming only the steps on the CPU.
    const rows = [...document.querySelectorAll('[data-part="accelerator"]')].map((r) => r.textContent);
    expect(rows).toEqual([
      "Intel Quick Sync (QSV)Decodes H.264, HEVC 10-bit · Encodes H.264, HEVC",
      "Intel VA-APIDecodes H.264 · Encodes H.264 · HDR tone mapping on CPU",
    ]);
    // The accelerator setting offers each accelerator, with the choice's
    // description in the open list.
    await user.click(screen.getByRole("button", { name: /Accelerator/ }));
    expect(await screen.findByRole("option", { name: "Automatic" })).toHaveTextContent("Uses Intel Quick Sync (QSV)");
    expect(screen.getByRole("option", { name: /^Intel VA-API/ })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: /NVIDIA/ })).not.toBeInTheDocument();
  });

  it("turns the accelerator setting off, and says so", async () => {
    const api = serveMockApi();
    onRequest("patch", "/settings", () => {
      api.settings = { ...api.settings, accelerator: "none", videoTranscodeLimit: 2 };
    });
    const { user } = renderApp("/admin/settings/transcoding");
    await user.click(await screen.findByRole("button", { name: /Accelerator/ }));
    await user.click(await screen.findByRole("option", { name: /^Off/ }));
    await user.click(screen.getByRole("button", { name: /Simultaneous video transcodes/ }));
    await user.click(await screen.findByRole("option", { name: "2" }));
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(sent("PATCH", "/settings")[0]?.body).toMatchObject({ accelerator: "none", videoTranscodeLimit: 2 }),
    );
    expect(
      await screen.findByText("Hardware acceleration is off. Video is transcoded on the CPU."),
    ).toBeInTheDocument();
  });
});
