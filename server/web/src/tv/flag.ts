/**
 * Whether this is the TV build, made with VITE_KIVO_TV=1. It is the same
 * app with the TV shell, spatial navigation and a remote's keys. The build
 * replaces it with a constant, so the web build drops what only the TV
 * runs. Tests of the TV build are named *.tv.test.*, which run with the
 * flag on. It lives with the TV code, which every layer may import, and
 * imports nothing itself.
 */
export const isTv = import.meta.env.VITE_KIVO_TV === "1";

/**
 * Whether this is the packaged TV app, made with VITE_KIVO_PACKAGED=1 beside
 * VITE_KIVO_TV=1. It is the TV build inside the Tizen package,
 * the only TV build there is: it keeps the server's address and its
 * session's token itself (src/tv/packaged.ts). Like isTv it is a constant,
 * so the web build and the TV flag's dev server drop what only the package runs. Its tests are
 * named *.packaged.test.*.
 */
export const isPackaged = import.meta.env.VITE_KIVO_PACKAGED === "1";
