import {
  NumberField as AriaNumberField,
  type NumberFieldProps as AriaNumberFieldProps,
  Input,
} from "react-aria-components";
import { FieldLabel, FieldNotes, type FieldVariant, fieldHeight, fieldInputClass } from "./FieldLabel";
import { useRowAria } from "./SettingsLayout";

type Props = Omit<AriaNumberFieldProps, "className" | "children"> & {
  variant?: FieldVariant;
  /** The label shown above the field. Without one, a field in a settings row is named by the row's label. */
  label?: string;
  description?: string;
  /** A server-side error for this field. */
  errorMessage?: string;
  /** An admin edited this field, so refreshes keep it. It shows a lock. */
  locked?: boolean;
};

/**
 * A number input, formatted for the locale. With `formatOptions` it can
 * show a percentage. In a settings row it is narrow and named by the row's
 * label.
 */
export function NumberField({
  variant = "settings",
  label,
  description,
  errorMessage,
  locked = false,
  ...props
}: Props) {
  const { isInvalid, ...row } = useRowAria(label !== undefined || props["aria-label"] !== undefined);
  const settings = variant === "settings";
  return (
    <AriaNumberField
      {...row}
      {...props}
      // Undefined rather than false when there's no error, so the field
      // still runs its own validation.
      isInvalid={errorMessage ? true : isInvalid}
      className={`flex flex-col gap-1.5 ${settings ? "w-28" : ""}`}
    >
      {label !== undefined && <FieldLabel label={label} locked={locked} />}
      <Input className={`${fieldHeight[variant]} ${fieldInputClass(variant)} ${settings ? "text-right" : ""}`} />
      <FieldNotes variant={variant} description={description} errorMessage={errorMessage} />
    </AriaNumberField>
  );
}
