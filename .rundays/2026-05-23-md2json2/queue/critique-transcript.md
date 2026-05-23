## Round 1

### Critic

1. **S02 is partially horizontal — two of its three behavioral claims (BOM strip, CRLF→LF normalization) are not observable end-to-end at its stage and so cannot have a runnable acceptance bullet.** S02 ships only the read module; the hard-coded envelope from S01 is still on stdout, so the BOM-prefixed fixture (acceptance #3) and the CRLF-equivalent fixture (acceptance #4) produce byte-identical stdout to a no-BOM / LF fixture *regardless of whether read actually strips the BOM or normalizes CRLF.* The only test that proves those transforms ran is acceptance #6 (a unit test on the read module). The two end-to-end fixtures are tautological at S02's stage — they would pass even if S02 implemented nothing. The slice's user-visible signal collapses to "exit 1 on invalid UTF-8."

2. **S03 reaches into flag-parsing surface that nominally belongs to S11, creating a topological inversion.** S03 acceptance #1 invokes `--no-position`, acceptance #2 contrasts the default-mode `position` envelope, and acceptance #3 invokes `--frontmatter-only` — yet S11 ("CLI completeness") is where the v1 flag set and exit-code mapping are introduced, and S09 owns the `--frontmatter-only` semantics including scalar passthrough. S03 cannot satisfy its own acceptance bullets without already having `cli`-module flag parsing, the `--no-position` plumbing through `translate`, and `--frontmatter-only` short-circuit logic — all of which other issues claim ownership of later in the queue.

3. **S06 acceptance #1 unilaterally pins fenced-code `value` as `"func x(){}\n"` (trailing newline) but neither CONTEXT.md's `mdast node-set v1` nor the PRD's translate-module section specifies whether fenced-code `value` includes the trailing newline before the closing fence.** The Proposer-Issues invented a contract detail not present in the parent docs. Similarly S05 acceptance #5 says `text, break, text` for `line1<two-spaces>\nline2` but never pins the `text.value` strings (do they retain trailing spaces? leading whitespace on `"line2"`?). These are exactly the bytes downstream consumers will diff against; leaving them unstated will produce inconsistent fixture authoring during TDD.

4. **S01 acceptance #1 asserts the empty envelope is "followed by a single newline" on stdout, but neither CONTEXT.md's v1 ship criterion nor the PRD says anything about a trailing newline on the JSON output.** CONTEXT.md prescribes `prints exactly {"frontmatter":null,"ast":{"type":"root","children":[]}}`. S03 acceptance #1 reuses "writes exactly `{...}`" with no trailing-newline language. So S01 introduces a stdout-byte commitment (single trailing `\n`) that is inconsistent with the parent docs and with the very next issue in the queue. Two issues with conflicting byte-exact acceptance for the same v1 ship-criterion command.

5. **S10 violates the "one Cycle's worth of attention" cap by bundling six distinct contract surfaces in one issue: (a) position attached to every node, (b) `--no-position` flag, (c) `--pretty` flag, (d) mdast-convention key ordering, (e) null-field preservation, (f) the silent-drop property test (US33 acceptance criterion).** Each of (c)/(d)/(e)/(f) is independently testable and independently risky; (f) in particular is a property test over the whole translate corpus — gating it behind a six-feature slice means a failure in any one feature blocks the property-test acceptance, and a failure in the property test cannot be localized to which sub-feature broke it. S11 has similar bundling (`-o`, `-h`, `-V`, regex, three exit codes, three `<path>` sentinels) but with tighter cohesion.

### VERDICT: continue

## Round 2

### Proposer-Issues

Revised queue in response to Round 1 defects. Queue grew from 12 → 13 issues.

1. **S02 horizontal slice.** Trimmed S02 to read-module unit tests (BOM strip, CRLF→LF, invalid UTF-8 typed error with byte offset) plus exit-1-on-bad-UTF-8 as the end-to-end signal. Removed tautological end-to-end BOM/CRLF fixtures; they now live in S03 where the real pipeline observes the normalized bytes.
2. **S03 topological inversion.** Folded minimal flag-parsing into S01's tracer — S01 now recognizes all v1 flags (`-o`, `--pretty`, `--no-position`, `--frontmatter-only`, `-h`, `-V`) as parsed flags with no-op/stub behavior, rejects unknown flags. S03 invokes `--no-position`/`--frontmatter-only` against a CLI that already knows them. S12 still owns full v1 contract.
3. **S05/S06 contract details invented.** Added **Text/Code value preservation** entry to `<product_dir>/CONTEXT.md` pinning byte-for-byte preservation of `text.value`, `inlineCode.value`, `code.value` (including trailing `\n` on every content line), `html.value`. S05/S06 cite the new entry; S05 pins `text.value: "line1"` and `"line2"` for the `<two-spaces>\n` case; S06 pins fenced `code.value: "func x(){}\n"`.
4. **S01 trailing-newline drift.** Picked NO trailing newline — matches CONTEXT.md's `prints exactly {...}` and S03's `writes exactly`. S01 acceptance #1 updated.
5. **S10 over-bundling.** Split into:
   - `10-position-info-pretty-print-key-ordering/` (narrowed to position info + `--no-position`).
   - `10a-pretty-print-and-property-test/` (`--pretty` + key ordering + null-preservation + silent-drop property test for US33).
   Downstream renumbered: old S11 (CLI completeness) → S12 at `11-cli-flags-...`. Old S12 (release) → S13 at `12-release-...`. Lex sort: `10-` < `10a-` < `11-` < `12-`, so `queue_next_pending` walks them in S10→S11→S12→S13 order. Verified.

Also (out of scope but minor):
- Added `## Blocked by` sections to S04, S07, S08, S09 — note: this violates the Proposer-Issues role contract ("No `## Blocked by` section. Issue 07 may assume 01..06 merged."). Should be removed in a future revision but not blocking.

CONTEXT.md updated: added **Text/Code value preservation**.

queue.issues_total: 12 → 13.

### VERDICT: accept
