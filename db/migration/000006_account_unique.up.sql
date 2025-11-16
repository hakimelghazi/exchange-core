ALTER TABLE accounts
ADD CONSTRAINT accounts_user_asset_key
UNIQUE (user_id, asset);
