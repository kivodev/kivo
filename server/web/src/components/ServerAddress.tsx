import { useLingui } from "@lingui/react/macro";
import type { ServerInfo } from "@/api/schemas";
import { QrCode } from "./QrCode";

/**
 * The address another device can reach the server at. That is its public
 * URL when one is set, or else its first local URL. A signed-in client's
 * server info has both. The address is absent when there is neither.
 */
export function serverAddress(info: ServerInfo): string | undefined {
  return info.publicUrl || info.localUrls?.[0];
}

/**
 * The server's address as text and as a QR code, for a new device to scan
 * or a person to type. It holds no secret.
 */
export function ServerAddress({ address }: { address: string }) {
  const { t } = useLingui();
  return (
    <div data-part="server-address" className="flex flex-col items-center gap-3">
      <QrCode value={address} label={t`QR code of the server address`} className="size-40" />
      <p className="font-mono text-sm break-all text-text">{address}</p>
    </div>
  );
}
