import { client } from "./client";
import type { CastToken } from "./schemas";

/** A cast token for Kivo's Web Receiver on the Cast device `receiverName`, such as "Living Room TV". */
export async function createCastToken(receiverName: string): Promise<CastToken> {
  const { data, error } = await client.POST("/api/v1/cast-tokens", {
    body: { receiverName: receiverName.slice(0, 100), receiverKind: "web" },
  });
  if (error) throw error;
  return data;
}
