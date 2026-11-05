// The TV build starts once, as its page loads. A test of the TV build runs
// in a page that the test before it stopped, so each starts it again.
import { beforeEach } from "vitest";
import { startTv } from "@/tv/start";

beforeEach(() => {
  startTv();
});
