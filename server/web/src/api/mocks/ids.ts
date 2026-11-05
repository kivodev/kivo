/** A mock UUIDv7 from a number, so fixed sample IDs read in order. */
export const mockId = (n: number) => `0190a000-0000-7000-8000-${n.toString(16).padStart(12, "0")}`;
