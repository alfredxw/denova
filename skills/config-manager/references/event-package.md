# Event package (`event_package`)

An event package is a Game-mode collection of reusable, schedulable event cards.

## Package shape

- `id`, `name`, `description`
- `events`: cards with stable `id`, `type_name`, `description_markdown`, `enabled`, `category`, `tags`, and `intensity`

Each `description_markdown` should cover:

- trigger scene
- how to fuse the event with current background
- rough beginning/development/turn/payoff logic
- recovery or lasting consequences
- reward or cost
- constraints that avoid forcing the player into one choice

When grounding cards in a work, inspect the relevant Lore first and use only facts actually read or supplied by the user. Cards should describe flexible situations, not fixed future chapter outlines. A balanced generated package usually contains 12–24 cards across categories relevant to the work.

Create/update the complete package, preserving unrequested cards and metadata. Attaching it to a Director is a separate `story_director` update. Use the latest `updated_at` revision for update/delete.
