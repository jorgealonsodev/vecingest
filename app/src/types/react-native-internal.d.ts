// react-native ships no type declarations for this internal module path.
// Test-only: `LoginScreen.theme.test.tsx` mocks it directly to control the
// desktop/mobile breakpoint deterministically (see that file's comment).
declare module "react-native/Libraries/Utilities/useWindowDimensions" {
  interface WindowDimensions {
    width: number;
    height: number;
    scale: number;
    fontScale: number;
  }
  const useWindowDimensions: () => WindowDimensions;
  export default useWindowDimensions;
}
