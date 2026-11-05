import { waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { bookIds } from "@/api/mocks/books";
import { renderApp, screen } from "@/test/render";
import { onRequest, sent, serveMockApi } from "@/test/server";

describe("Edit fields for a book", () => {
  it("shows the book's fields and sends only those changed, under book", async () => {
    const api = serveMockApi();
    // The server locks what an edit changes.
    onRequest("patch", "/works/{id}", () => {
      const churn = api.works[bookIds.churn];
      if (churn) churn.lockedFields = ["book.asin", "book.narrators", "book.series"];
    });
    const { user } = renderApp(`/works/${bookIds.churn}`);
    await user.click(await screen.findByRole("button", { name: "More actions for The Churn" }));
    await user.click(within(await screen.findByRole("menu")).getByRole("menuitem", { name: "Edit" }));
    const dialog = await screen.findByRole("dialog", { name: "Edit The Churn" });
    expect(within(dialog).getByRole("textbox", { name: "Authors" })).toHaveValue("James S. A. Corey");
    expect(within(dialog).getByRole("textbox", { name: "Series name" })).toHaveValue("The Expanse");
    expect(within(dialog).getByRole("textbox", { name: "Book" })).toHaveValue("1.5");
    expect(within(dialog).getByRole("textbox", { name: "Release date" })).toHaveValue("2014");

    const narrators = within(dialog).getByRole("textbox", { name: "Narrators" });
    await user.clear(narrators);
    await user.type(narrators, "Ray Porter{Enter}Jefferson Mays");
    await user.click(within(dialog).getByRole("button", { name: "Add series" }));
    const names = within(dialog).getAllByRole("textbox", { name: "Series name" });
    await user.type(names[1] as HTMLElement, "The Expanse Novellas");
    await user.type(within(dialog).getAllByRole("textbox", { name: "Book" })[1] as HTMLElement, "1");
    await user.type(within(dialog).getByRole("textbox", { name: "ASIN" }), "B00H8NDG7S");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(sent("PATCH", "/works/{id}").at(-1)?.body).toEqual({
      book: {
        asin: "B00H8NDG7S",
        narrators: ["Ray Porter", "Jefferson Mays"],
        series: [
          { name: "The Expanse", position: "1.5", primary: true },
          { name: "The Expanse Novellas", position: "1", primary: false },
        ],
      },
    });

    await user.click(screen.getByRole("button", { name: "More actions for The Churn" }));
    await user.click(within(await screen.findByRole("menu")).getByRole("menuitem", { name: "Edit" }));
    const again = await screen.findByRole("dialog", { name: "Edit The Churn" });
    expect(within(again).getByRole("textbox", { name: "Narrators (locked)" })).toBeInTheDocument();
  });
});
