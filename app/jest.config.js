/** @type {import('jest').Config} */
module.exports = {
  preset: "jest-expo",
  setupFiles: ["./jest.setup.js"],
  collectCoverageFrom: ["src/**/*.{ts,tsx}"],
};
