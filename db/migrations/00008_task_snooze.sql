-- +goose Up

-- Wiedervorlage: an open task can be put aside until a date. Until then it
-- is not in the list of open tasks.
ALTER TABLE task ADD COLUMN snoozed_until timestamptz;

-- +goose Down
ALTER TABLE task DROP COLUMN snoozed_until;
