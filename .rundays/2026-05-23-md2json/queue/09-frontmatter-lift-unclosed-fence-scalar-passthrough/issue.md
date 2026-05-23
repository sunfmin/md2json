# S09: Lift YAML frontmatter onto the envelope; honor unclosed-fence and malformed-frontmatter rules

Status: ready-for-agent

The parse stage detects a `---`-fenced YAML block at the very top of the normalized document, YAML-parses it, lifts the value to the envelope's `frontmatter` field, and strips the block from the bytes handed to goldmark for body parsing. The unclosed-fence rule lands here: an opening `---` on line 1 with no matching closing fence is *not* frontmatter — the whole document parses as body and `frontmatter` stays `null`, exit `0`. Malformed YAML between closed fences (tab indent, unbalanced quotes, duplicate keys, etc.) is a hard error emitting `md2json: <path>:<line>:<col>: invalid frontmatter: <yaml error>` and exit `1`. The `--frontmatter-only` flag emits the frontmatter value at the top level of stdout, with the scalar passthrough rule: a scalar YAML value (`--- "hello" ---`, `--- 42 ---`, `--- null ---`) emits the scalar's JSON equivalent at top level (`"hello"`, `42`, `null`) — never wrapped in an object.

## Acceptance

- [ ] Fixture: a document opening with `---\ntitle: x\n---\n\nbody` produces an envelope whose `frontmatter` is `{"title":"x"}` and whose `ast` parses `body` as a paragraph.
- [ ] Fixture: a document opening with `---\ntitle: x\n` (no closing fence) produces `frontmatter: null` and parses the whole document — including the opening `---` line — as body content, exit `0`.
- [ ] Fixture: a document with closed `---` fences containing malformed YAML (e.g. unbalanced quotes) writes `md2json: <path>:<line>:<col>: invalid frontmatter: <yaml error>` to stderr, exits `1`, with nothing on stdout.
- [ ] Fixture: `md2json --frontmatter-only` on `--- "hello" ---` writes exactly `"hello"` (plus newline) to stdout, exit `0`.
- [ ] Fixture: `md2json --frontmatter-only` on `--- 42 ---` writes `42`; on `--- null ---` writes `null`; on a document with no frontmatter writes `null`.
- [ ] Fixture: `md2json --frontmatter-only` on the malformed-YAML document emits the same `invalid frontmatter` stderr line and exit `1` (the flag does not change the upstream error contract).

## Blocked by

S08 — needs the full body-translation node set in place so frontmatter-plus-body fixtures cover realistic inputs.
