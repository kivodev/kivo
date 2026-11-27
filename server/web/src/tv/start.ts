import { startBackKeys, stopBackKeys } from "./back";
import { forgetFocus } from "./focusMemory";
import { startSpatial, stopSpatial } from "./spatial";
import { registerMediaKeys } from "./tizen";
import { logWebosHdr } from "./webos";

let started = false;

/**
 * Starts the TV build: data-tv on the root, which switches the tokens to
 * the TV scale (tv.web in shared/tokens.json), the Back keys, the media
 * keys in the packaged app, and spatial navigation. On an LG TV it logs the
 * panel's HDR answers for testing. Calling it again does nothing.
 */
export function startTv() {
  if (started) return;
  started = true;
  document.documentElement.dataset.tv = "";
  startBackKeys();
  registerMediaKeys();
  startSpatial();
  void logWebosHdr();
}

/** Stops the TV build, for tests. */
export function stopTv() {
  if (!started) return;
  started = false;
  stopBackKeys();
  stopSpatial();
  forgetFocus();
  delete document.documentElement.dataset.tv;
}
