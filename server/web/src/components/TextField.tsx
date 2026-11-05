import {
  TextField as AriaTextField,
  type TextFieldProps as AriaTextFieldProps,
  Input,
  TextArea,
} from "react-aria-components";
import { FieldLabel, FieldNotes, type FieldVariant, fieldHeight, fieldInputClass } from "./FieldLabel";
import { useRowAria } from "./SettingsLayout";

type Props = Omit<AriaTextFieldProps, "className" | "children"> & {
  variant?: FieldVariant;
  /** The label shown above the field. Without one, a field in a settings row is named by the row's label. */
  label?: string;
  description?: string;
  /** A server-side error for this field. */
  errorMessage?: string;
  /** Several lines, such as an overview. */
  multiline?: boolean;
  /** An admin edited this field, so refreshes keep it. It shows a lock. */
  locked?: boolean;
  placeholder?: string;
};

/**
 * A text field, such as a name, a key or an address. In a settings row it
 * has a set width and sits beside the label, or under it when it doesn't
 * fit. It is named by the row's label and described by its help and error.
 */
export function TextField({
  variant = "settings",
  label,
  description,
  errorMessage,
  multiline = false,
  locked = false,
  placeholder,
  ...props
}: Props) {
  const { isInvalid, ...row } = useRowAria(label !== undefined || props["aria-label"] !== undefined);
  const input = fieldInputClass(variant);
  // The TV's type is large, so its fields are taller.
  const height = `${fieldHeight[variant]} tv:h-12`;
  return (
    <AriaTextField
      {...row}
      {...props}
      // Undefined rather than false when there's no error, so the field
      // still runs its own validation.
      isInvalid={errorMessage ? true : isInvalid}
      className={`flex flex-col gap-1.5 ${variant === "settings" ? "w-(--layout-settings-field-width) max-w-full" : ""}`}
    >
      {label !== undefined && <FieldLabel label={label} locked={locked} />}
      {multiline ? (
        <TextArea rows={4} placeholder={placeholder} className={`py-2 ${input}`} />
      ) : (
        <Input placeholder={placeholder} className={`${height} ${input}`} />
      )}
      <FieldNotes variant={variant} description={description} errorMessage={errorMessage} />
    </AriaTextField>
  );
}
