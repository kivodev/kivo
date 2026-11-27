// Leaving Kivo on a TV. The packaged Tizen app (clients/tizen) has Tizen's
// application exit and uses it. The packaged webOS app (clients/webos) and
// the TV flag's dev server, in a browser, have no Tizen APIs; closing the
// window ends the webOS app.

type TvGlobals = {
  tizen?: { application: { getCurrentApplication(): { exit(): void } } };
};

/**
 * Leaves Kivo for the TV's home screen: Tizen's application exit where the
 * page has it, or else closing the window, which a browser tab it didn't
 * open ignores.
 */
export function exitApp() {
  const { tizen } = window as unknown as TvGlobals;
  if (tizen) tizen.application.getCurrentApplication().exit();
  else window.close();
}
