import { Redirect } from "expo-router";
import Head from "expo-router/head";

/**
 * M0 has only the login screen. Every other route group in PRD §7.2's
 * structure ((owner), (admin), (company), (superadmin)) arrives with its
 * own milestone.
 *
 * Web `<title>` via `expo-router/head`'s `<Head>` — see
 * `(auth)/login.tsx`'s comment on why `Stack.Screen options.title` does not
 * work here. This route redirects immediately, but the browser can still
 * render this tab's title for an instant — and Expo Router's static export
 * prerenders it — so it must not be left empty either.
 */
export default function Index() {
  return (
    <>
      <Head>
        <title>Vecingest</title>
      </Head>
      <Redirect href="/(auth)/login" />
    </>
  );
}
