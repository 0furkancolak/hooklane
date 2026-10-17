import { useCallback, useMemo, useState } from "react";
import { ApiClient } from "../shared/api/client";
import { ApiContext } from "../shared/api/context";
import { clearKey, loadKey, saveKey } from "../shared/session";
import { SignInPage } from "../features/auth/SignInPage";
import { Layout } from "./Layout";

export default function App() {
  const [key, setKey] = useState<string | null>(() => loadKey());
  const [notice, setNotice] = useState<string | undefined>();

  const signOut = useCallback((message?: string) => {
    clearKey();
    setKey(null);
    setNotice(message);
  }, []);

  const client = useMemo(
    () => (key ? new ApiClient(key, { onUnauthorized: () => signOut("Your API key was rejected. It may have been revoked; sign in again.") }) : null),
    [key, signOut],
  );

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
      <Layout onSignOut={() => signOut()} />
    </ApiContext.Provider>
  );
}
