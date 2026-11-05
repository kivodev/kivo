import { Trans } from "@lingui/react/macro";
import { useRef, useState } from "react";
import { Input, Text, TextField } from "react-aria-components";
import { Button } from "./Button";
import { FieldLabel } from "./FieldLabel";

/**
 * A value to copy, such as a new API key, shown read-only with a Copy
 * button. When the browser keeps the clipboard from the page, as it does
 * on a plain http address, Copy selects the value and says to press
 * Ctrl+C.
 */
export function CopyField({ label, value }: { label: string; value: string }) {
  const input = useRef<HTMLInputElement>(null);
  const [copied, setCopied] = useState<"yes" | "selected" | undefined>(undefined);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied("yes");
    } catch {
      input.current?.select();
      setCopied("selected");
    }
  };
  return (
    <TextField value={value} isReadOnly className="flex flex-col gap-1.5">
      <FieldLabel label={label} />
      <div className="flex gap-2">
        <Input
          ref={input}
          onFocus={(e) => e.currentTarget.select()}
          className="h-10 min-w-0 flex-1 rounded-md border border-border bg-surface px-3 font-mono text-sm text-text outline-none data-focused:border-accent pointer-coarse:text-base"
        />
        <Button onPress={() => void copy()}>{copied === "yes" ? <Trans>Copied</Trans> : <Trans>Copy</Trans>}</Button>
      </div>
      {copied === "selected" && (
        <Text slot="description" role="status" className="text-xs text-text-muted">
          <Trans>Press Ctrl+C to copy.</Trans>
        </Text>
      )}
    </TextField>
  );
}
