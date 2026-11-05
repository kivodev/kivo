// Norigin, the spatial navigation library, in the TV build only. The flag
// is read here directly rather than through flag.ts, so the web build folds
// it as it reads this module and drops Norigin whole.
import * as Norigin from "@noriginmedia/norigin-spatial-navigation";

/** Norigin in the TV build. Call it only there: elsewhere it throws. */
export function norigin(): typeof Norigin {
  if (import.meta.env.VITE_KIVO_TV !== "1") throw new Error("Norigin is for the TV build only");
  return Norigin;
}
