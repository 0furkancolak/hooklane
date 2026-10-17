import { createHmac, timingSafeEqual } from "node:crypto";

const toleranceSeconds = 5 * 60;

/** Verifies a Standard Webhooks signature over the raw body bytes. */
export function verify(secretValue, headers, rawBody, nowSeconds = Math.floor(Date.now() / 1000)) {
  const id = headers["webhook-id"];
  const timestamp = headers["webhook-timestamp"];
  const signatures = headers["webhook-signature"];
  if (!id || !timestamp || !signatures) return false;
  if (!/^\d+$/.test(timestamp) || Math.abs(nowSeconds - Number(timestamp)) > toleranceSeconds) return false;

  const key = Buffer.from(secretValue.replace(/^whsec_/, ""), "base64");
  const expected = createHmac("sha256", key).update(`${id}.${timestamp}.`).update(rawBody).digest();
  return signatures.split(" ").some((candidate) => {
    const [version, encoded] = candidate.split(",", 2);
    if (version !== "v1" || !encoded) return false;
    const got = Buffer.from(encoded, "base64");
    return got.length === expected.length && timingSafeEqual(got, expected);
  });
}
