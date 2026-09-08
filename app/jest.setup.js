// React 19 requires test environments to opt in explicitly; without this,
// state updates inside `act()` still warn and RNTL's render/query bookkeeping
// becomes unreliable across tests in the same file.
globalThis.IS_REACT_ACT_ENVIRONMENT = true;

// Native module mocks for the RNTL/jest-expo environment. Real behaviour is
// exercised on-device; these tests assert the app calls the correct API
// (`expo-secure-store`, never `AsyncStorage`), not the native implementation.
jest.mock("expo-secure-store", () => ({
  setItemAsync: jest.fn().mockResolvedValue(undefined),
  getItemAsync: jest.fn().mockResolvedValue(null),
  deleteItemAsync: jest.fn().mockResolvedValue(undefined),
}));
