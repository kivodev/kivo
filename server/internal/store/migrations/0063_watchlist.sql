-- users ----------------------------------------------------------------------
-- My List is now called Watchlist. A name is unique per user, so a list a
-- user already named Watchlist takes the built-in list's old name, which
-- only the built-in list could hold. The built-in list steps aside first,
-- under a name no user has, because the swap checks one row at a time.
UPDATE list SET name = 'watchlist-migrating-' || hex(id) WHERE built_in;

UPDATE list SET name = 'My List' WHERE NOT built_in AND name = 'Watchlist';

UPDATE list SET name = 'Watchlist' WHERE built_in;
