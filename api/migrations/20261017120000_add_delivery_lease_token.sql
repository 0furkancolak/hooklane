-- +goose Up

-- Each claim gets a fresh lease token. Completion must present the token, so a
-- worker whose lease expired and was reclaimed cannot overwrite the new owner.
ALTER TABLE deliveries ADD COLUMN lease_token UUID;

ALTER TABLE deliveries ADD CONSTRAINT deliveries_lease_consistency CHECK (
    (status = 'processing') = (lease_token IS NOT NULL AND locked_until IS NOT NULL)
);

-- +goose Down

ALTER TABLE deliveries DROP CONSTRAINT IF EXISTS deliveries_lease_consistency;
ALTER TABLE deliveries DROP COLUMN IF EXISTS lease_token;
