import { waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { renderApp } from "@/test/render";
import { serveMockApi } from "@/test/server";

const { longPress, confirm } = vi.hoisted(() => ({ longPress: vi.fn(), confirm: vi.fn() }));
vi.mock("react-aria", async (original) => {
  const actual = await original<typeof import("react-aria")>();
  return {
    ...actual,
    useLongPress: (...args: Parameters<typeof actual.useLongPress>) => {
      longPress();
      return actual.useLongPress(...args);
    },
  };
});
vi.mock("@/components/ConfirmDialog", () => ({
  ConfirmDialog: () => {
    confirm();
    return null;
  },
}));

describe("WorkCard on TV", () => {
  it("mounts cards without web touch handlers or closed confirmation dialogs", async () => {
    serveMockApi();
    const { container } = renderApp("/");
    await waitFor(() => expect(container.querySelectorAll('[data-part="card"]').length).toBeGreaterThan(0));
    expect(longPress).not.toHaveBeenCalled();
    expect(confirm).not.toHaveBeenCalled();
  });
});
