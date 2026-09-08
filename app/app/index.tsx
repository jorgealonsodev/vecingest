import { Redirect } from "expo-router";

/**
 * M0 has only the login screen. Every other route group in PRD §7.2's
 * structure ((owner), (admin), (company), (superadmin)) arrives with its
 * own milestone.
 */
export default function Index() {
  return <Redirect href="/(auth)/login" />;
}
