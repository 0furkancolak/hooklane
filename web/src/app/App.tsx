import { useCallback, useMemo, useState } from "react";
import { ApiClient } from "../shared/api/client";
import { ApiContext } from "../shared/api/context";
import { t } from "../shared/i18n";
import { useRoute } from "../shared/router";
import { clearKey, loadKey, saveKey } from "../shared/session";
import { SignInPage } from "../features/auth/SignInPage";
import { PortalPage } from "../features/portal/PortalPage";
import { Layout } from "./Layout";

export default function App() {
  const route = useRoute();
  // The customer portal has its own credential and never uses the admin key.
  if (route.name === "portal") {
    return <PortalPage token={route.token} />;
  }
  return <AdminApp />;
}

function AdminApp() {
  const [key, setKey] = useState<string | null>(() => loadKey());
  const [notice, setNotice] = useState<string | undefined>();

  const signOut = useCallback((message?: string) => {
    clearKey();
    setKey(null);
    setNotice(message);
  }, []);

  const client = useMemo(() => (key ? new ApiClient(key, { onUnauthorized: () => signOut(t("app.keyRejected")) }) : null), [key, signOut]);

  if (!client) {
    return (
      <SignInPage
        notice={notice}
        onSignedIn={(value) => {
          saveKey(value);
          setNotice(undefined);
          setKey(value);
        }}
      />
    );
  }
  return (
    <ApiContext.Provider value={client}>
      <Layout
        onSignOut={() => {
          // Ending the session on the server makes the token useless if it leaked.
          if (client.isSession) void client.signOutSession().catch(() => undefined);
          signOut();
        }}
      />
    </ApiContext.Provider>
  );
}
