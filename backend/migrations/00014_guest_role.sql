-- +goose Up

-- Guest checkout: a purchase made with only a phone number, no email or
-- password. Rather than making users.email/password_hash nullable — which
-- would ripple into every non-pointer u.Email scan in postgres/users.go and
-- postgres/revenue.go — a guest gets a synthetic, deterministic email
-- (guest+<phone>@guest.katafa.internal) and a random, never-derivable
-- password hash. users.email stays NOT NULL UNIQUE, and that uniqueness is
-- what gives find-or-create-by-phone its ON CONFLICT for free.

ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users
    ADD CONSTRAINT users_role_check
    CHECK (role IN ('user','analyst','admin','guest'));

-- +goose Down

ALTER TABLE users DROP CONSTRAINT users_role_check;
ALTER TABLE users
    ADD CONSTRAINT users_role_check
    CHECK (role IN ('user','analyst','admin'));
