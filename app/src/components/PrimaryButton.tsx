import { Button, type ButtonProps } from "react-native-paper";
import { MIN_TOUCH_TARGET } from "../theme";

/**
 * Shared primary (contained) action button for the auth screens, matching
 * `docs/design/vecingest-web.md`'s shape scale: 8px radius for
 * buttons/inputs/controls. Full-pill radius is reserved for chips/badges.
 *
 * `Button.tsx` computes `borderRadius = 5 * theme.roundness` for MD3
 * buttons (20px at the app's default `roundness` of 4) unless the caller's
 * `style` sets its own `borderRadius`, which it reads first — so this
 * overrides it per instance instead of touching the global `roundness`
 * (which would also reshape the TextInput outline, Card, Chip, etc.).
 *
 * `style` is destructured out before spreading the rest of the props so an
 * empty caller spread can't clobber these defaults, while a caller that
 * does pass `style` still merges on top via the array form.
 */
export function PrimaryButton({ style, ...rest }: ButtonProps) {
  return (
    <Button
      mode="contained"
      {...rest}
      style={[
        {
          borderRadius: 8,
          minHeight: MIN_TOUCH_TARGET,
          justifyContent: "center",
        },
        style,
      ]}
    />
  );
}
