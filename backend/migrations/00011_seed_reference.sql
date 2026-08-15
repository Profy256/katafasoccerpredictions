-- +goose Up

-- market_types and packages are lookup tables keyed by their natural code, and
-- their content is part of the product contract: these rows are the source the
-- frontend's MARKETS map in src/lib/markets.ts duplicates for label rendering.
-- Seeded in a migration rather than by a job, because a missing market_types
-- row breaks a foreign key on every prediction insert.

INSERT INTO market_types (code, display_name, short_name, tab_label, slug, outcomes, sort_order) VALUES
('ONE_X_TWO', 'Match Result (1X2)', '1X2', 'Match Result', '1x2',
 '[{"value":"HOME","label":"Home Win","shortLabel":"1"},
   {"value":"DRAW","label":"Draw","shortLabel":"X"},
   {"value":"AWAY","label":"Away Win","shortLabel":"2"}]'::jsonb, 1),

('DOUBLE_CHANCE', 'Double Chance', 'DC', 'Double Chance', 'double-chance',
 '[{"value":"1X","label":"Home or Draw","shortLabel":"1X"},
   {"value":"12","label":"Home or Away","shortLabel":"12"},
   {"value":"X2","label":"Draw or Away","shortLabel":"X2"}]'::jsonb, 2),

('BTTS', 'Both Teams To Score', 'BTTS', 'Both Teams Score', 'btts',
 '[{"value":"YES","label":"Yes","shortLabel":"Yes"},
   {"value":"NO","label":"No","shortLabel":"No"}]'::jsonb, 3),

('OVER_UNDER_1_5', 'Over / Under 1.5 Goals', 'O/U 1.5', 'Over/Under 1.5', 'over-under-1-5',
 '[{"value":"OVER","label":"Over 1.5","shortLabel":"O 1.5"},
   {"value":"UNDER","label":"Under 1.5","shortLabel":"U 1.5"}]'::jsonb, 4),

('OVER_UNDER_2_5', 'Over / Under 2.5 Goals', 'O/U 2.5', 'Over/Under 2.5', 'over-under-2-5',
 '[{"value":"OVER","label":"Over 2.5","shortLabel":"O 2.5"},
   {"value":"UNDER","label":"Under 2.5","shortLabel":"U 2.5"}]'::jsonb, 5),

('OVER_UNDER_3_5', 'Over / Under 3.5 Goals', 'O/U 3.5', 'Over/Under 3.5', 'over-under-3-5',
 '[{"value":"OVER","label":"Over 3.5","shortLabel":"O 3.5"},
   {"value":"UNDER","label":"Under 3.5","shortLabel":"U 3.5"}]'::jsonb, 6);

-- typical_price_ugx is indicative only; the real figure is whatever the admin
-- set on each individual slip.
INSERT INTO packages (code, name, tagline, description, typical_price_ugx, highlights, sort_order) VALUES
('ordinary', 'Ordinary', 'The daily working slip',
 'A longer slip of everyday value selections across the leagues we cover. The entry point to the analysts’ work.',
 2000,
 '["Around five selections per slip","Published every matchday morning","Mixed markets across all covered leagues"]'::jsonb, 1),

('vip', 'VIP', 'Fewer legs, higher conviction',
 'A tighter slip. The analysts drop anything they are not confident in, so there are fewer selections and more reasoning behind each one.',
 5000,
 '["Three selections per slip","Written reasoning on every pick","Posted at least four hours before kickoff"]'::jsonb, 2),

('akatambula', 'AKATAMBULA', 'The one they stake themselves',
 'The flagship slip, assembled by hand and released only when the analysts agree it is worth putting out. Some days there is no Akatambula at all.',
 20000,
 '["Two selections, maximum conviction","Hand-entered by the admin, never auto-generated","Not published every day"]'::jsonb, 3);

-- +goose Down
DELETE FROM packages WHERE code IN ('ordinary','vip','akatambula');
DELETE FROM market_types WHERE code IN ('ONE_X_TWO','DOUBLE_CHANCE','BTTS',
                                        'OVER_UNDER_1_5','OVER_UNDER_2_5','OVER_UNDER_3_5');
