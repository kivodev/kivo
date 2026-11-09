import { Button as AriaButton, type ButtonProps as AriaButtonProps } from "react-aria-components";

export type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
export type ButtonSize = "md" | "sm" | "icon";

const variants: Record<ButtonVariant, string> = {
  primary: "bg-accent text-on-accent hover:bg-accent-hover",
  secondary: "bg-surface-raised text-text hover:bg-surface-hover border border-border",
  ghost: "text-text hover:bg-surface-hover",
  // On TV it isn't filled until it has focus (index.css), so it can't look
  // focused beside the safe choice that has.
  danger: "bg-danger text-on-accent hover:bg-danger/90 tv:border tv:border-border tv:bg-surface-raised tv:text-danger",
};

const sizes: Record<ButtonSize, string> = {
  md: "h-10 px-4 gap-2 text-sm font-semibold",
  sm: "h-8 px-3 gap-1.5 text-xs font-semibold",
  icon: "size-10 justify-center",
};

/** Classes for anything that looks like a button, such as a router link. */
export function buttonClass(variant: ButtonVariant = "secondary", size: ButtonSize = "md"): string {
  return [
    // Focus rings come from the base layer's :focus-visible rule.
    "inline-flex shrink-0 cursor-pointer items-center justify-center rounded-md transition-colors select-none touch-target",
    "disabled:opacity-50 data-disabled:opacity-50 data-disabled:cursor-default",
    variants[variant],
    // A toggle that's on, such as My List. The attribute selector outranks
    // the variant's text colour wherever either lands in the stylesheet.
    "aria-pressed:text-accent",
    sizes[size],
  ].join(" ");
}

type Props = Omit<AriaButtonProps, "className"> & {
  variant?: ButtonVariant;
  size?: ButtonSize;
  className?: string;
};

export function Button({ variant = "secondary", size = "md", className = "", ...props }: Props) {
  return <AriaButton {...props} className={`${buttonClass(variant, size)} ${className}`} />;
}
