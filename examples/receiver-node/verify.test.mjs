// node --test examples/receiver-node/
import assert from "node:assert/strict";
import { test } from "node:test";

import { verify } from "./verify.mjs";

test("Standard Webhooks test vector", () => {
  const secret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw";
  const headers = {
    "webhook-id": "msg_p5jXN8AQM9LWM0D4loKWxJek",
    "webhook-timestamp": "1614265330",
    "webhook-signature": "v1,bogus v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE=",
  };
  const body = Buffer.from('{"test": 2432232314}');
  assert.equal(verify(secret, headers, body, 1614265330 + 60), true);
  assert.equal(verify(secret, headers, Buffer.from('{"test": 1}'), 1614265330 + 60), false);
  assert.equal(verify(secret, headers, body, 1614265330 + 3600), false);
});
