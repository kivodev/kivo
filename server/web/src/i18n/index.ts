import { i18n } from "@lingui/core";
import { messages as en } from "./locales/en/messages.po";

// English is the source locale. More catalogs are added as they are
// translated.
i18n.load({ en });
i18n.activate("en");

export { i18n };
