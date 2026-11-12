import { expect, it } from "vitest";
import { identityHeaders } from "./identity";

const samsung =
  "Mozilla/5.0 (SMART-TV; LINUX; Tizen 9.0) AppleWebKit/537.36 (KHTML, like Gecko) 120.0.6099.5/9.0 TV Safari/537.36";
const lg =
  "Mozilla/5.0 (Web0S; Linux/SmartTV) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/132.0.6834.207 Safari/537.36 WebAppManager";

it("says it's the Samsung TV app on a Samsung TV", () => {
  expect(identityHeaders(samsung)).toMatchObject({ "Kivo-App": "kivo-tizen", "Kivo-Platform": "tizen" });
});

it("says it's the LG TV app on an LG TV", () => {
  expect(identityHeaders(lg)).toMatchObject({ "Kivo-App": "kivo-webos", "Kivo-Platform": "webos" });
});
