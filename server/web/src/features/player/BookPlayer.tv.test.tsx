import { fireEvent, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { bookIds } from "@/api/mocks/books";
import { audio, miniPlayer } from "@/test/player";
import { renderApp, screen } from "@/test/render";
import { serveMockApi } from "@/test/server";

describe("Audio player on a TV", () => {
  it("takes the remote's media keys while docked", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${bookIds.leviathan}`);
    await user.click(await screen.findByRole("button", { name: "Play" }));
    const dock = () => miniPlayer() as HTMLElement;
    await waitFor(() => expect(within(dock()).getByRole("button", { name: "Pause" })).toBeEnabled());
    audio().currentTime = 60;

    // Tizen's Fast forward and Rewind, by key code.
    fireEvent.keyDown(window, { key: "Unidentified", keyCode: 417 });
    expect(audio().currentTime).toBe(90);
    fireEvent.keyDown(window, { key: "Unidentified", keyCode: 412 });
    expect(audio().currentTime).toBe(80);

    // Play/Pause.
    fireEvent.keyDown(window, { key: "Unidentified", keyCode: 10252 });
    await waitFor(() => expect(within(dock()).getByRole("button", { name: "Play" })).toBeInTheDocument());
  });

  it("moves by chapter on the remote's previous and next", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${bookIds.leviathan}`);
    await user.click(await screen.findByRole("button", { name: "Play" }));
    await waitFor(() =>
      expect(within(miniPlayer() as HTMLElement).getByRole("button", { name: "Pause" })).toBeEnabled(),
    );

    // Tizen's next, by key code: the second file's start.
    fireEvent.keyDown(window, { key: "Unidentified", keyCode: 10233 });
    await waitFor(() => expect(audio().getAttribute("src")).toMatch(/files\/1$/));
    // Previous, at the chapter's start, goes to the chapter before.
    fireEvent.keyDown(window, { key: "Unidentified", keyCode: 10232 });
    await waitFor(() => expect(audio().getAttribute("src")).toMatch(/files\/0$/));
  });

  it("leaves a media key the video player took", async () => {
    serveMockApi();
    const { user } = renderApp(`/works/${bookIds.leviathan}`);
    await user.click(await screen.findByRole("button", { name: "Play" }));
    await waitFor(() =>
      expect(within(miniPlayer() as HTMLElement).getByRole("button", { name: "Pause" })).toBeEnabled(),
    );
    audio().currentTime = 60;
    const take = (e: KeyboardEvent) => e.preventDefault();
    window.addEventListener("keydown", take, { capture: true });
    fireEvent.keyDown(window, { key: "Unidentified", keyCode: 417 });
    window.removeEventListener("keydown", take, { capture: true });
    expect(audio().currentTime).toBe(60);
  });
});
