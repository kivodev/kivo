import type { Problem } from "./schemas";

/** Whether a thrown query or mutation error is the server's problem document. */
export function isProblem(error: unknown): error is Problem {
  return (
    typeof error === "object" &&
    error !== null &&
    typeof (error as Problem).code === "string" &&
    typeof (error as Problem).status === "number"
  );
}

/**
 * Whether a response means the server itself didn't answer. That is a
 * gateway in front of it answering 502, 503 or 504 without a problem
 * document, to say the server is down. The server's own problems, its 503s
 * included, count as answers.
 */
export function gatewayDown(status: number, body: unknown): boolean {
  return status >= 502 && status <= 504 && !isProblem(body);
}

/**
 * Whether a thrown error means no answer came from the server. Either the
 * request failed to send, or a gateway said the server is down. The client
 * turns every other answer into a problem document, with code `unknown`
 * when it wasn't one.
 */
function unanswered(error: unknown): boolean {
  return !isProblem(error);
}

/** The problem code of an error, or `network` when the request never got an answer. */
export function errorCode(error: unknown): string {
  return isProblem(error) ? error.code : "network";
}

/**
 * Queries retry twice when no answer came. They never retry on an answer,
 * whether a 4xx or one of the server's own 5xx problems.
 */
export function shouldRetry(failureCount: number, error: unknown): boolean {
  return failureCount < 2 && unanswered(error);
}

/**
 * A grid page retries five times when no answer came. Its cards have
 * nothing else to show, and a dropped connection can take more than a few
 * seconds to come back.
 */
export function shouldRetryPage(failureCount: number, error: unknown): boolean {
  return failureCount < 5 && unanswered(error);
}

/** The delay before a grid page's next try: 1 s, 2 s, 4 s, 8 s and 16 s after each failure. */
export function retryPageDelay(failureCount: number): number {
  return Math.min(1000 * 2 ** failureCount, 30_000);
}
