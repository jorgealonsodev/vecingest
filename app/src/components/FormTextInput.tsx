import { TextInput, type TextInputProps, useTheme } from "react-native-paper";

/**
 * Shared outlined text input for every form field in the auth screens,
 * matching `docs/design/vecingest-web.md`'s "Text Inputs & Selects": white
 * surface background, 8px outline radius, `#D6DAD5` border, accent focus
 * outline.
 *
 * Two react-native-paper defaults fight this design on their own:
 * - `TextInputOutlined` falls back to `theme.colors.background` (the cream
 *   page color) for its background whenever the caller's flattened `style`
 *   doesn't set one, instead of `theme.colors.surface` (white) — reads as a
 *   beige patch on a white card.
 * - `Outline` defaults its radius to `theme.roundness` (4), not the 8px the
 *   design reserves for inputs/buttons/controls.
 *
 * `style`/`outlineStyle` are destructured out before spreading the rest of
 * the props so an empty caller spread can't clobber these defaults, while a
 * caller that does pass `style`/`outlineStyle` (e.g. `minHeight`) still
 * merges on top via the array form, non-destructively.
 */
export function FormTextInput({
  style,
  outlineStyle,
  ...rest
}: TextInputProps) {
  const theme = useTheme();

  return (
    <TextInput
      mode="outlined"
      outlineColor={theme.colors.outline}
      activeOutlineColor={theme.colors.primary}
      {...rest}
      style={[{ backgroundColor: theme.colors.surface }, style]}
      outlineStyle={[{ borderRadius: 8 }, outlineStyle]}
    />
  );
}
