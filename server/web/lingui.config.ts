import { defineConfig } from "@lingui/cli";
import { formatter } from "@lingui/format-po";

export default defineConfig({
  sourceLocale: "en",
  locales: ["en"],
  catalogs: [
    {
      path: "<rootDir>/src/i18n/locales/{locale}/messages",
      include: ["<rootDir>/src"],
      exclude: ["**/*.test.*", "<rootDir>/src/api/mocks/**", "<rootDir>/src/test/**"],
    },
  ],
  // Origins without line numbers keep the catalog stable when code moves,
  // so CI can check it is current with a plain diff.
  format: formatter({ lineNumbers: false }),
  orderBy: "messageId",
});
